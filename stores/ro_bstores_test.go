package stores_test

import (
	"context"
	"path/filepath"
	"testing"

	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/stores"

	"github.com/filecoin-project/dagstore"

	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"
)

func TestReadOnlyStoreTracker(t *testing.T) {
	ctx := context.Background()

	// Create a CARv2 file from a fixture
	testData := tut.NewLibp2pTestData(ctx, t)
	fpath1 := filepath.Join("retrievalmarket", "impl", "fixtures", "lorem.txt")
	_, carFilePath := testData.LoadUnixFSFileToStore(t, fpath1)
	fpath2 := filepath.Join("retrievalmarket", "impl", "fixtures", "lorem_under_1_block.txt")
	_, carFilePath2 := testData.LoadUnixFSFileToStore(t, fpath2)
	rdOnlyBS1, err := blockstore.OpenReadOnly(carFilePath, carv2.ZeroLengthSectionAsEOF(true), blockstore.UseWholeCIDs(true))
	require.NoError(t, err)
	len1 := getBstoreLen(ctx, t, rdOnlyBS1)

	k1 := "k1"
	k2 := "k2"
	tracker := stores.NewReadOnlyBlockstores()

	// Get a non-existent key
	_, err = tracker.Get(k1)
	require.True(t, stores.IsNotFound(err))

	// Add a read-only blockstore
	ok, err := tracker.Track(k1, rdOnlyBS1)
	require.NoError(t, err)
	require.True(t, ok)

	// Get the blockstore using its key
	got, err := tracker.Get(k1)
	require.NoError(t, err)

	// Verify the blockstore is the same
	lenGot := getBstoreLen(ctx, t, got)
	require.Equal(t, len1, lenGot)

	// Call GetOrOpen using the same key
	got2, err := tracker.GetOrOpen(k1, carFilePath)
	require.NoError(t, err)

	// Verify the blockstore is the same
	lenGot2 := getBstoreLen(ctx, t, got2)
	require.Equal(t, len1, lenGot2)

	// Call GetOrOpen with a different CAR file
	rdOnlyBS2, err := tracker.GetOrOpen(k2, carFilePath2)
	require.NoError(t, err)

	// Verify the blockstore is different
	len2 := getBstoreLen(ctx, t, rdOnlyBS2)
	require.NotEqual(t, len1, len2)

	// Untrack the second blockstore from the tracker
	err = tracker.Untrack(k2)
	require.NoError(t, err)

	// Verify it's been removed
	_, err = tracker.Get(k2)
	require.True(t, stores.IsNotFound(err))
}

func getBstoreLen(ctx context.Context, t *testing.T, bs dagstore.ReadBlockstore) int {
	ch, err := bs.AllKeysChan(ctx)
	require.NoError(t, err)
	var len int
	for range ch {
		len++
	}
	return len
}
