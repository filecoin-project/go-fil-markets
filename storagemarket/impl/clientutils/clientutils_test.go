package clientutils_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
)

func TestCommP(t *testing.T) {
	ctx := context.Background()
	proofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	t.Run("when PieceCID is present on data ref", func(t *testing.T) {
		pieceCid := &shared_testutil.GenerateCids(1)[0]
		pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
		var storeID *multistore.StoreID
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			PieceCid:     pieceCid,
			PieceSize:    pieceSize,
		}
		respcid, ressize, err := clientutils.CommP(ctx, nil, proofType, data, storeID)
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
		allSelector := shared.AllSelector()

		t.Run("when pieceIO succeeds", func(t *testing.T) {
			pieceCid := shared_testutil.GenerateCids(1)[0]
			pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
			storeID := multistore.StoreID(4)
			pieceIO := &testPieceIO{t, proofType, root, allSelector, &storeID, pieceCid, pieceSize, nil}
			respcid, ressize, err := clientutils.CommP(ctx, pieceIO, proofType, data, &storeID)
			require.NoError(t, err)
			require.Equal(t, respcid, pieceCid)
			require.Equal(t, ressize, pieceSize)
		})

		t.Run("when storeID is not present", func(t *testing.T) {
			pieceCid := shared_testutil.GenerateCids(1)[0]
			pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
			pieceIO := &testPieceIO{t, proofType, root, allSelector, nil, pieceCid, pieceSize, nil}
			respcid, ressize, err := clientutils.CommP(ctx, pieceIO, proofType, data, nil)
			require.NoError(t, err)
			require.Equal(t, respcid, pieceCid)
			require.Equal(t, ressize, pieceSize)
		})

		t.Run("when pieceIO fails", func(t *testing.T) {
			expectedMsg := "something went wrong"
			storeID := multistore.StoreID(4)
			pieceIO := &testPieceIO{t, proofType, root, allSelector, &storeID, cid.Undef, 0, errors.New(expectedMsg)}
			respcid, ressize, err := clientutils.CommP(ctx, pieceIO, proofType, data, &storeID)
			require.EqualError(t, err, fmt.Sprintf("generating CommP: %s", expectedMsg))
			require.Equal(t, respcid, cid.Undef)
			require.Equal(t, ressize, abi.UnpaddedPieceSize(0))
		})
	})
}

type testPieceIO struct {
	t                  *testing.T
	expectedRt         abi.RegisteredSealProof
	expectedPayloadCid cid.Cid
	expectedSelector   ipld.Node
	expectedStoreID    *multistore.StoreID
	pieceCID           cid.Cid
	pieceSize          abi.UnpaddedPieceSize
	err                error
}

func (t *testPieceIO) GeneratePieceCommitment(rt abi.RegisteredSealProof, payloadCid cid.Cid, selector ipld.Node, storeID *multistore.StoreID, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (cid.Cid, abi.UnpaddedPieceSize, error) {
	require.Equal(t.t, rt, t.expectedRt)
	require.Equal(t.t, payloadCid, t.expectedPayloadCid)
	require.Equal(t.t, selector, t.expectedSelector)
	require.Equal(t.t, storeID, t.expectedStoreID)
	return t.pieceCID, t.pieceSize, t.err
}

func (t *testPieceIO) GeneratePieceReader(cid.Cid, ipld.Node, *multistore.StoreID, ...car.OnNewCarBlockFunc) (io.ReadCloser, uint64, error, <-chan error) {
	panic("not implemented")
}

func (t *testPieceIO) ReadPiece(storeID *multistore.StoreID, r io.Reader) (cid.Cid, error) {
	panic("not implemented")
}

func TestLabelField(t *testing.T) {
	payloadCID := shared_testutil.GenerateCids(1)[0]
	label, err := clientutils.LabelField(payloadCID)
	require.NoError(t, err)
	resultCid, err := cid.Decode(label)
	require.NoError(t, err)
	require.True(t, payloadCID.Equals(resultCid))
}
