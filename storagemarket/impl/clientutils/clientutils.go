package clientutils

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

func CommP(ctx context.Context, pieceIO pieceio.PieceIO, rt abi.RegisteredProof, data *storagemarket.DataRef) (cid.Cid, abi.UnpaddedPieceSize, error) {
	if data.PieceCid != nil {
		return *data.PieceCid, data.PieceSize, nil
	}
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	// entire DAG selector
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	commp, paddedSize, err := pieceIO.GeneratePieceCommitment(rt, data.Root, allSelector)
	if err != nil {
		return cid.Undef, 0, xerrors.Errorf("generating CommP: %w", err)
	}

	return commp, paddedSize, nil
}

type VerifyFunc func(crypto.Signature, address.Address, []byte) bool

func VerifyResponse(resp network.SignedResponse, minerAddr address.Address, verifier VerifyFunc) error {
	b, err := cborutil.Dump(&resp.Response)
	if err != nil {
		return err
	}
	verified := verifier(*resp.Signature, minerAddr, b)
	if !verified {
		return xerrors.New("could not verify signature")
	}
	return nil
}
