package clientutils_test

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
)

func TestCommP(t *testing.T) {
	ctx := context.Background()
	t.Run("when PieceCID is present on data ref", func(t *testing.T) {
		pieceCid := &shared_testutil.GenerateCids(1)[0]
		pieceSize := abi.UnpaddedPieceSize(rand.Uint64())
		data := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			PieceCid:     pieceCid,
			PieceSize:    pieceSize,
		}
		respcid, ressize, err := clientutils.CommP(ctx, "", data)
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

		t.Run("when CARv2 file path is not present", func(t *testing.T) {
			respcid, ressize, err := clientutils.CommP(ctx, "", data)
			require.Error(t, err)
			require.Contains(t, err.Error(), "need Carv2 file path")
			require.Equal(t, cid.Undef, respcid)
			require.EqualValues(t, 0, ressize)
		})
	})
}

func TestCommPGeneration(t *testing.T) {
	carV1Path := filepath.Join("storagemarket", "fixtures", "test.car")
	ctx := context.Background()
	root, CARv2Path := shared_testutil.GenCARV2(t, carV1Path)
	require.NotEmpty(t, CARv2Path)
	defer os.Remove(CARv2Path)

	data := &storagemarket.DataRef{
		TransferType: storagemarket.TTGraphsync,
		Root:         root,
	}

	respcid, _, err := clientutils.CommP(ctx, CARv2Path, data)
	require.NoError(t, err)
	require.NotEqual(t, respcid, cid.Undef)

	// TODO Generate CommP with the same file again -> should match.

	// TODO Generate CommP with a different file -> should not match.
}

func TestLabelField(t *testing.T) {
	payloadCID := shared_testutil.GenerateCids(1)[0]
	label, err := clientutils.LabelField(payloadCID)
	require.NoError(t, err)
	resultCid, err := cid.Decode(label)
	require.NoError(t, err)
	require.True(t, payloadCID.Equals(resultCid))
}
