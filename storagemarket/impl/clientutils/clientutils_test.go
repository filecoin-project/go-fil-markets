package clientutils_test

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

	file1 := filepath.Join("storagemarket", "fixtures", "payload.txt")
	file2 := filepath.Join("storagemarket", "fixtures", "payload2.txt")

	commp1 := genCommPFromFile(t, ctx, file1)
	commP2 := genCommPFromFile(t, ctx, file2)

	commP3 := genCommPFromFile(t, ctx, file1)
	commP4 := genCommPFromFile(t, ctx, file2)

	// commP matches for the same files but is different for different files.
	require.Equal(t, commp1, commP3)
	require.Equal(t, commP2, commP4)
	require.NotEqual(t, commp1, commP2)
	require.NotEqual(t, commP3, commP4)
}

func genCommPFromFile(t *testing.T, ctx context.Context, filePath string) cid.Cid {
	root, CARv2Path, _ := shared_testutil.GenCARv2FromNormalFile(t, filePath)
	require.NotEmpty(t, CARv2Path)
	defer os.Remove(CARv2Path)
	data := &storagemarket.DataRef{
		TransferType: storagemarket.TTGraphsync,
		Root:         root,
	}

	respcid, _, err := clientutils.CommP(ctx, CARv2Path, data)
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

// TODO This test fails right now but should be green when the CARv2 bug is fixed.
func TestNoDuplicatesInCARv2(t *testing.T) {
	// The CARv2 file for a UnixFS DAG that has duplicates should NOT have duplicates.
	file1 := filepath.Join("storagemarket", "fixtures", "duplicate_blocks.txt")
	root, CARv2Path, bstore := shared_testutil.GenCARv2FromNormalFile(t, file1)
	require.NotEmpty(t, CARv2Path)
	defer os.Remove(CARv2Path)

	v2r, err := carv2.NewReaderMmap(CARv2Path)
	require.NoError(t, err)
	defer v2r.Close()

	// Get a reader over the CARv1 payload of the CARv2 file.
	cr, err := car.NewCarReader(v2r.CarV1Reader())
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

	// A CARv1 traversal over the UnixFS DAG wll return all the de-duped blocks -> should be the same as what the CARv1 reader above returned.
	seen2 := make(map[cid.Cid]struct{})
	var mu sync.Mutex

	sc := car.NewSelectiveCar(context.Background(), bstore, []car.Dag{
		{
			Root:     root,
			Selector: shared.AllSelector(),
		},
	})

	sc.Write(ioutil.Discard, func(b car.Block) error {
		mu.Lock()
		defer mu.Unlock()

		if _, ok := seen2[b.BlockCID]; ok {
			err = xerrors.Errorf("already seen cid %s", b.BlockCID)
		}

		seen2[b.BlockCID] = struct{}{}

		return nil
	})

	mu.Lock()
	defer mu.Unlock()

	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(seen, seen2))
}
