package blockunsealing

import (
	"bytes"
	"context"
	"io"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/piecestore"
)

// LoaderWithUnsealing is an ipld.Loader function that will also unseal pieces as needed
type LoaderWithUnsealing interface {
	Load(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error)
}

type loaderWithUnsealing struct {
	ctx             context.Context
	bs              blockstore.Blockstore
	pieceInfo       piecestore.PieceInfo
	carIO           pieceio.CarIO
	unsealer        UnsealingFunc
	alreadyUnsealed bool
}

// UnsealingFunc is a function that unseals sectors at a given offset and length
type UnsealingFunc func(ctx context.Context, sectorId uint64, offset uint64, length uint64) (io.ReadCloser, error)

// NewLoaderWithUnsealing creates a loader that will attempt to read blocks from the blockstore but unseal the piece
// as needed using the passed unsealing function
func NewLoaderWithUnsealing(ctx context.Context, bs blockstore.Blockstore, pieceInfo piecestore.PieceInfo, carIO pieceio.CarIO, unsealer UnsealingFunc) LoaderWithUnsealing {
	return &loaderWithUnsealing{ctx, bs, pieceInfo, carIO, unsealer, false}
}

func (lu *loaderWithUnsealing) Load(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
	cl, ok := lnk.(cidlink.Link)
	if !ok {
		return nil, xerrors.New("Unsupported link type")
	}
	c := cl.Cid
	// check if intermediate blockstore has cid
	has, err := lu.bs.Has(c)
	if err != nil {
		return nil, xerrors.Errorf("attempting to load cid from blockstore: %w", err)
	}

	// attempt unseal if block is not in blockstore
	if !has && !lu.alreadyUnsealed {
		err = lu.attemptUnseal()
		if err != nil {
			return nil, err
		}
	}

	blk, err := lu.bs.Get(c)
	if err != nil {
		return nil, xerrors.Errorf("attempting to load cid from blockstore: %w", err)
	}

	return bytes.NewReader(blk.RawData()), nil
}

func (lu *loaderWithUnsealing) attemptUnseal() error {

	lu.alreadyUnsealed = true

	// try to unseal data from piece
	var reader io.ReadCloser
	var err error
	for _, deal := range lu.pieceInfo.Deals {
		reader, err = lu.unsealer(lu.ctx, deal.SectorID, deal.Offset, deal.Length)
		if err == nil {
			break
		}
	}

	// no successful unseal
	if err != nil {
		return xerrors.Errorf("Unable to unseal piece: %w", err)
	}

	// attempt to load data as a car file into the block store
	_, err = lu.carIO.LoadCar(lu.bs, reader)
	if err != nil {
		return xerrors.Errorf("attempting to read Car file: %w", err)
	}

	return nil
}
