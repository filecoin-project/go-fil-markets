package retrievalimpl

import (
	"context"
	"testing"

	ds "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestLazyBlockstoreGet(t *testing.T) {
	b := shared_testutil.GenerateBlocksOfSize(1, 1024)[0]

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)
	err := bs.Put(b)
	require.NoError(t, err)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	blk, err := lbs.Get(b.Cid())
	require.NoError(t, err)
	require.Equal(t, b, blk)
}

func TestLazyBlockstoreDelete(t *testing.T) {
	b := shared_testutil.GenerateBlocksOfSize(1, 1024)[0]

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)
	err := bs.Put(b)
	require.NoError(t, err)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	err = lbs.DeleteBlock(b.Cid())
	require.NoError(t, err)

	has, err := bs.Has(b.Cid())
	require.NoError(t, err)
	require.False(t, has)
}

func TestLazyBlockstoreAllKeysChan(t *testing.T) {
	blks := shared_testutil.GenerateBlocksOfSize(2, 1024)

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)

	for _, b := range blks {
		err := bs.Put(b)
		require.NoError(t, err)
	}

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	ch, err := lbs.AllKeysChan(context.Background())
	require.NoError(t, err)

	var count int
	for k := range ch {
		count++
		has, err := bs.Has(k)
		require.NoError(t, err)
		require.True(t, has)
	}
	require.Len(t, blks, count)
}

func TestLazyBlockstoreHas(t *testing.T) {
	b := shared_testutil.GenerateBlocksOfSize(1, 1024)[0]

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)
	err := bs.Put(b)
	require.NoError(t, err)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	has, err := lbs.Has(b.Cid())
	require.NoError(t, err)
	require.True(t, has)
}

func TestLazyBlockstoreGetSize(t *testing.T) {
	b := shared_testutil.GenerateBlocksOfSize(1, 1024)[0]

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)
	err := bs.Put(b)
	require.NoError(t, err)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	sz, err := lbs.GetSize(b.Cid())
	require.NoError(t, err)
	require.Equal(t, 1024, sz)
}

func TestLazyBlockstorePut(t *testing.T) {
	b := shared_testutil.GenerateBlocksOfSize(1, 1024)[0]

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	err := lbs.Put(b)
	require.NoError(t, err)

	blk, err := bs.Get(b.Cid())
	require.NoError(t, err)
	require.Equal(t, b, blk)
}

func TestLazyBlockstorePutMany(t *testing.T) {
	blks := shared_testutil.GenerateBlocksOfSize(2, 1024)

	ds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(ds)

	lbs := newLazyBlockstore(func() (bstore.Blockstore, error) {
		return bs, nil
	})

	err := lbs.PutMany(blks)
	require.NoError(t, err)

	for _, b := range blks {
		blk, err := bs.Get(b.Cid())
		require.NoError(t, err)
		require.Equal(t, b, blk)
	}
}
