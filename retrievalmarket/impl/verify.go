package retrievalimpl

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared/params"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	pb "github.com/ipfs/go-unixfs/pb"
	"github.com/ipld/go-ipld-prime"
	dagpb "github.com/ipld/go-ipld-prime-proto"
	free "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"
)

type BlockVerifier interface {
	Verify(context.Context, blocks.Block) (done bool, err error)
}

type OptimisticVerifier struct {
}

func (o *OptimisticVerifier) Verify(context.Context, blocks.Block) (bool, error) {
	// It's probably fine
	return false, nil
}

type blockResponse struct {
	done bool
	err  error
}

type SelectorVerifier struct {
	root        ipld.Link
	initiated   bool
	inputBlocks chan blocks.Block
	responses   chan blockResponse
}

func NewSelectorVerifier(root ipld.Link) BlockVerifier {
	return &SelectorVerifier{root, false, nil, nil}
}

func (sv *SelectorVerifier) handleError(ctx context.Context, err error) {
	_ = sv.writeBlockResponse(ctx, false, err)
	close(sv.inputBlocks)
	close(sv.responses)
}

func (sv *SelectorVerifier) writeBlockResponse(ctx context.Context, done bool, err error) error {
	select {
	case <-ctx.Done():
		return errors.New("Context cancelled")
	case sv.responses <- blockResponse{done, err}:
	}
	return nil
}

func (sv *SelectorVerifier) readBlockResponse(ctx context.Context) (bool, error) {
	select {
	case <-ctx.Done():
		return false, errors.New("Context cancelled")
	case br := <-sv.responses:
		return br.done, br.err
	}
}

func (sv *SelectorVerifier) writeBlock(ctx context.Context, blk blocks.Block) error {
	select {
	case <-ctx.Done():
		return errors.New("Context cancelled")
	case sv.inputBlocks <- blk:
	}
	return nil
}

func (sv *SelectorVerifier) readBlock(ctx context.Context) (blocks.Block, error) {
	select {
	case <-ctx.Done():
		return nil, errors.New("Context cancelled")
	case blk := <-sv.inputBlocks:
		return blk, nil
	}
}

func (sv *SelectorVerifier) runTraversal(ctx context.Context) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var chooser traversal.NodeBuilderChooser = dagpb.AddDagPBSupportToChooser(func(ipld.Link, ipld.LinkContext) ipld.NodeBuilder {
		return free.NodeBuilder()
	})
	loader := func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
		err := sv.writeBlockResponse(subCtx, false, nil)
		if err != nil {
			return nil, err
		}
		blk, err := sv.readBlock(subCtx)
		if err != nil {
			return nil, err
		}
		c := lnk.(cidlink.Link).Cid
		if !c.Equals(blk.Cid()) {
			return nil, retrievalmarket.ErrVerification
		}
		return bytes.NewBuffer(blk.RawData()), nil
	}
	nd, err := sv.root.Load(subCtx, ipld.LinkContext{}, chooser(sv.root, ipld.LinkContext{}), loader)
	if err != nil {
		sv.handleError(subCtx, err)
		return
	}
	ssb := builder.NewSelectorSpecBuilder(free.NodeBuilder())

	allSelector, err := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Selector()
	if err != nil {
		sv.handleError(subCtx, err)
		return
	}
	err = traversal.Progress{
		Cfg: &traversal.Config{
			Ctx:                    subCtx,
			LinkLoader:             loader,
			LinkNodeBuilderChooser: chooser,
		},
	}.WalkAdv(nd, allSelector, func(traversal.Progress, ipld.Node, traversal.VisitReason) error { return nil })
	if err != nil {
		sv.handleError(subCtx, err)
		return
	}
	_ = sv.writeBlockResponse(subCtx, true, nil)
	close(sv.inputBlocks)
	close(sv.responses)
}

func (sv *SelectorVerifier) Verify(ctx context.Context, blk blocks.Block) (done bool, err error) {
	if !sv.initiated {
		sv.initiated = true
		sv.inputBlocks = make(chan blocks.Block, 1)
		sv.responses = make(chan blockResponse, 1)
		go sv.runTraversal(ctx)
		done, err := sv.readBlockResponse(ctx)
		if err != nil {
			return done, err
		}
	}
	err = sv.writeBlock(ctx, blk)
	if err != nil {
		return false, err
	}
	return sv.readBlockResponse(ctx)
}

type UnixFs0Verifier struct {
	Root    cid.Cid
	rootBlk blocks.Block

	expect int
	seen   int

	sub *UnixFs0Verifier
}

func (b *UnixFs0Verifier) verify(ctx context.Context, blk blocks.Block) (last bool, internal bool, err error) {
	if b.sub != nil {
		// TODO: check links here (iff b.sub.sub == nil)

		subLast, internal, err := b.sub.verify(ctx, blk)
		if err != nil {
			return false, false, err
		}
		if subLast {
			b.sub = nil
			b.seen++
		}

		return b.seen == b.expect, internal, nil
	}

	if b.seen >= b.expect { // this is probably impossible
		return false, false, xerrors.New("unixfs verifier: too many nodes in level")
	}

	links, err := b.checkInternal(blk)
	if err != nil {
		return false, false, err
	}

	if links > 0 { // TODO: check if all links are intermediate (or all aren't)
		if links > params.UnixfsLinksPerLevel {
			return false, false, xerrors.New("unixfs verifier: too many links in intermediate node")
		}

		if b.seen+1 == b.expect && links != params.UnixfsLinksPerLevel {
			return false, false, xerrors.New("unixfs verifier: too few nodes in level")
		}

		b.sub = &UnixFs0Verifier{
			Root:    blk.Cid(),
			rootBlk: blk,
			expect:  links,
		}

		// don't mark as seen yet
		return false, true, nil
	}

	b.seen++
	return b.seen == b.expect, false, nil
}

func (b *UnixFs0Verifier) checkInternal(blk blocks.Block) (int, error) {
	nd, err := ipldformat.Decode(blk)
	if err != nil {
		log.Warnf("IPLD Decode failed: %s", err)
		return 0, err
	}

	// TODO: check size
	switch nd := nd.(type) {
	case *merkledag.ProtoNode:
		fsn, err := unixfs.FSNodeFromBytes(nd.Data())
		if err != nil {
			log.Warnf("unixfs.FSNodeFromBytes failed: %s", err)
			return 0, err
		}
		if fsn.Type() != pb.Data_File {
			return 0, xerrors.New("internal nodes must be a file")
		}
		if len(fsn.Data()) > 0 {
			return 0, xerrors.New("internal node with data")
		}
		if len(nd.Links()) == 0 {
			return 0, xerrors.New("internal node with no links")
		}
		return len(nd.Links()), nil

	case *merkledag.RawNode:
		return 0, nil
	default:
		return 0, xerrors.New("verifier: unknown node type")
	}
}

func (b *UnixFs0Verifier) Verify(ctx context.Context, blk blocks.Block) (bool, error) {
	// root is special
	if b.rootBlk == nil {
		if !b.Root.Equals(blk.Cid()) {
			return false, xerrors.Errorf("unixfs verifier: root block CID didn't match: valid %s, got %s", b.Root, blk.Cid())
		}
		b.rootBlk = blk
		links, err := b.checkInternal(blk)
		if err != nil {
			return false, err
		}

		b.expect = links
		return links == 0, nil
	}

	done, _, err := b.verify(ctx, blk)
	return done, err
}

var _ BlockVerifier = &OptimisticVerifier{}
var _ BlockVerifier = &UnixFs0Verifier{}
