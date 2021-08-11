package clientutils_test

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
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

func TestCommPSuccess(t *testing.T) {
	ctx := context.Background()

	file1 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/payload.txt")
	file2 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/payload2.txt")

	// ----------------
	// commP for file 1.
	root1, f1FullCAR := shared_testutil.CreateDenseCARv2(t, file1)
	defer os.Remove(f1FullCAR)

	root2, f1FileStoreCAR := shared_testutil.CreateRefCARv2(t, file1)
	defer os.Remove(f1FileStoreCAR)

	// assert the two files have different contents
	assertFileDifferent(t, f1FullCAR, f1FileStoreCAR)

	// but the same DAG Root.
	require.Equal(t, root1, root2)

	// commPs match for both since it's the same Unixfs DAG.
	commpf1Full := genCommPFromCARFile(t, ctx, root1, f1FullCAR)
	commpf1Filestore := genCommPFromCARFile(t, ctx, root2, f1FileStoreCAR)
	require.EqualValues(t, commpf1Full, commpf1Filestore)

	// ------------
	// commP for file2.
	root1, f2FullCAR := shared_testutil.CreateDenseCARv2(t, file2)
	defer os.Remove(f2FullCAR)

	root2, f2FileStoreCAR := shared_testutil.CreateRefCARv2(t, file2)
	defer os.Remove(f2FileStoreCAR)

	// assert the two files have different contents
	assertFileDifferent(t, f2FullCAR, f2FileStoreCAR)

	// but the same DAG Root.
	require.Equal(t, root1, root2)

	// commPs match for both since it's the same Unixfs DAG.
	commpf2Full := genCommPFromCARFile(t, ctx, root1, f2FullCAR)
	commpf2Filestore := genCommPFromCARFile(t, ctx, root2, f2FileStoreCAR)
	require.EqualValues(t, commpf2Full, commpf2Filestore)

	// However -> commP's are different across different files/DAGs.
	require.NotEqualValues(t, commpf1Full, commpf2Full)
	require.NotEqualValues(t, commpf1Filestore, commpf2Filestore)

}

func assertFileDifferent(t *testing.T, f1Path string, f2Path string) {
	f1, err := os.Open(f1Path)
	require.NoError(t, err)
	defer f1.Close()

	f2, err := os.Open(f2Path)
	require.NoError(t, err)
	defer f2.Close()

	bzf1, err := ioutil.ReadAll(f1)
	require.NoError(t, err)

	bzf2, err := ioutil.ReadAll(f2)
	require.NoError(t, err)

	require.NotEqualValues(t, bzf1, bzf2)
}

func genCommPFromCARFile(t *testing.T, ctx context.Context, root cid.Cid, carFilePath string) cid.Cid {
	data := &storagemarket.DataRef{
		TransferType: storagemarket.TTGraphsync,
		Root:         root,
	}

	respcid, _, err := clientutils.CommP(ctx, carFilePath, data)
	require.NoError(t, err)
	require.NotEqual(t, respcid, cid.Undef)

	return respcid
}

func TestLabelField(t *testing.T) {
	payloadCID := shared_testutil.GenerateCids(1)[0]
	label, err := clientutils.LabelField(payloadCID)
	require.NoError(t, err)
	resultCid, err := cid.Decode(label)
	require.NoError(t, err)
	require.True(t, payloadCID.Equals(resultCid))
}

func TestNoDuplicatesInCARv2(t *testing.T) {
	// The CARv2 file for a UnixFS DAG that has duplicates should NOT have duplicates.
	file1 := filepath.Join(shared_testutil.ThisDir(t), "../../fixtures/duplicate_blocks.txt")
	_, path := shared_testutil.CreateDenseCARv2(t, file1)
	require.NotEmpty(t, path)
	defer os.Remove(path)

	v2r, err := carv2.OpenReader(path)
	require.NoError(t, err)
	defer v2r.Close()

	// Get a reader over the CARv1 payload of the CARv2 file.
	cr, err := car.NewCarReader(v2r.DataReader())
	require.NoError(t, err)

	seen := make(map[cid.Cid]struct{})
	for {
		b, err := cr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		_, ok := seen[b.Cid()]
		require.Falsef(t, ok, "already seen cid %s", b.Cid())
		seen[b.Cid()] = struct{}{}
	}
}
