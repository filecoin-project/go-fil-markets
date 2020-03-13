package clientutils

import (
	"context"
	"github.com/ipfs/go-cid"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"
	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// CommP calculates the commP for a given dataref
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

// VerifyFunc is a function that can validate a signature for a given address and bytes
type VerifyFunc func(crypto.Signature, address.Address, []byte) bool

// VerifyResponse verifies the signature on the given signed response matches
// the given miner address, using the given signature verification function
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
