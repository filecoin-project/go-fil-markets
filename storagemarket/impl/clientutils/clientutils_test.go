package clientutils_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

func TestCommP(t *testing.T) {
	ctx := context.Background()
	proofType := abi.RegisteredProof_StackedDRG2KiBPoSt
	t.Run("when PieceCID is present on data ref", func(t *testing.T) {
		pieceCid := &shared_testutil.GenerateCids(1)[0]
		pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			PieceCid:     pieceCid,
			PieceSize:    pieceSize,
		}
		respcid, ressize, err := clientutils.CommP(ctx, nil, proofType, data)
		require.NoError(t, err)
		require.Equal(t, respcid, *pieceCid)
		require.Equal(t, ressize, pieceSize)
	})

	t.Run("when PieceCID is not present on data ref", func(t *testing.T) {
		root := shared_testutil.GenerateCids(1)[0]
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         root,
		}
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
		allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
		t.Run("when pieceIO succeeds", func(t *testing.T) {
			pieceCid := shared_testutil.GenerateCids(1)[0]
			pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
			pieceIO := &testPieceIO{t, proofType, root, allSelector, pieceCid, pieceSize, nil}
			respcid, ressize, err := clientutils.CommP(ctx, pieceIO, proofType, data)
			require.NoError(t, err)
			require.Equal(t, respcid, pieceCid)
			require.Equal(t, ressize, pieceSize)
		})

		t.Run("when pieceIO fails", func(t *testing.T) {
			expectedMsg := "something went wrong"
			pieceIO := &testPieceIO{t, proofType, root, allSelector, cid.Undef, 0, errors.New(expectedMsg)}
			respcid, ressize, err := clientutils.CommP(ctx, pieceIO, proofType, data)
			require.EqualError(t, err, fmt.Sprintf("generating CommP: %s", expectedMsg))
			require.Equal(t, respcid, cid.Undef)
			require.Equal(t, ressize, abi.UnpaddedPieceSize(0))
		})
	})
}

func TestVerifyResponse(t *testing.T) {
	tests := map[string]struct {
		sresponse network.SignedResponse
		verifier  clientutils.VerifyFunc
		shouldErr bool
	}{
		"successful verification": {
			sresponse: shared_testutil.MakeTestStorageNetworkSignedResponse(),
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return true, nil
			},
			shouldErr: false,
		},
		"bad response": {
			sresponse: network.SignedResponse{
				Response:  network.Response{},
				Signature: shared_testutil.MakeTestSignature(),
			},
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return true, nil
			},
			shouldErr: true,
		},
		"verification fails": {
			sresponse: shared_testutil.MakeTestStorageNetworkSignedResponse(),
			verifier: func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error) {
				return false, nil
			},
			shouldErr: true,
		},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			err := clientutils.VerifyResponse(context.Background(), data.sresponse, address.TestAddress, shared.TipSetToken{}, data.verifier)
			require.Equal(t, err != nil, data.shouldErr)
		})
	}
}

type testPieceIO struct {
	t                  *testing.T
	expectedRt         abi.RegisteredProof
	expectedPayloadCid cid.Cid
	expectedSelector   ipld.Node
	pieceCID           cid.Cid
	pieceSize          abi.UnpaddedPieceSize
	err                error
}

func (t *testPieceIO) GeneratePieceCommitment(rt abi.RegisteredProof, payloadCid cid.Cid, selector ipld.Node) (cid.Cid, abi.UnpaddedPieceSize, error) {
	require.Equal(t.t, rt, t.expectedRt)
	require.Equal(t.t, payloadCid, t.expectedPayloadCid)
	require.Equal(t.t, selector, t.expectedSelector)
	return t.pieceCID, t.pieceSize, t.err
}

func (t *testPieceIO) ReadPiece(r io.Reader) (cid.Cid, error) {
	panic("not implemented")
}
