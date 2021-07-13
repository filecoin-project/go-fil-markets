package carstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/carstore"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestReadOnlyStoreTracker(t *testing.T) {
	ctx := context.Background()

	// Create a CARv2 file from a fixture
	testData := tut.NewLibp2pTestData(ctx, t)
	fpath1 := filepath.Join("retrievalmarket", "impl", "fixtures", "lorem.txt")
	_, carFilePath := testData.LoadUnixFSFileToStore(t, fpath1)
	fpath2 := filepath.Join("retrievalmarket", "impl", "fixtures", "lorem_under_1_block.txt")
	_, carFilePath2 := testData.LoadUnixFSFileToStore(t, fpath2)
	rdOnlyBS1, err := blockstore.OpenReadOnly(carFilePath)
	require.NoError(t, err)
	len1 := getBstoreLen(ctx, t, rdOnlyBS1)

	k1 := "k1"
	k2 := "k2"
	tracker := carstore.NewReadOnlyStoreTracker()

	// Get a non-existent key
	_, err = tracker.Get(k1)
	require.True(t, carstore.IsNotFound(err))

	// Add a read-only blockstore
	ok, err := tracker.Add(k1, rdOnlyBS1)
	require.NoError(t, err)
	require.True(t, ok)

	// Get the blockstore using its key
	got, err := tracker.Get(k1)
	require.NoError(t, err)

	// Verify the blockstore is the same
	lenGot := getBstoreLen(ctx, t, got)
	require.Equal(t, len1, lenGot)

	// Call GetOrCreate using the same key
	got2, err := tracker.GetOrCreate(k1, carFilePath)
	require.NoError(t, err)

	// Verify the blockstore is the same
	lenGot2 := getBstoreLen(ctx, t, got2)
	require.Equal(t, len1, lenGot2)

	// Call GetOrCreate with a different CAR file
	rdOnlyBS2, err := tracker.GetOrCreate(k2, carFilePath2)
	require.NoError(t, err)

	// Verify the blockstore is different
	len2 := getBstoreLen(ctx, t, rdOnlyBS2)
	require.NotEqual(t, len1, len2)

	// Clean the second blockstore from the tracker
	err = tracker.CleanBlockstore(k2)
	require.NoError(t, err)

	// Verify it's been removed
	_, err = tracker.Get(k2)
	require.True(t, carstore.IsNotFound(err))
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
