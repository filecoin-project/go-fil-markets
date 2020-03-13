package storedcounter_test

import (
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storedcounter"
)

func TestStoredCounter(t *testing.T) {
	ds := datastore.NewMapDatastore()

	t.Run("test two instances with same data store and key count together", func(t *testing.T) {
		key := datastore.NewKey("counter")
		sc1 := storedcounter.New(ds, key)
		next, err := sc1.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(0))

		sc2 := storedcounter.New(ds, key)
		next, err = sc2.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(1))

		next, err = sc1.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(2))

		next, err = sc2.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(3))
	})

	t.Run("test two instances with same data store but different keys count seperate", func(t *testing.T) {

		key1 := datastore.NewKey("counter 1")
		key2 := datastore.NewKey("counter 2")

		sc1 := storedcounter.New(ds, key1)
		next, err := sc1.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(0))

		sc2 := storedcounter.New(ds, key2)
		next, err = sc2.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(0))

		next, err = sc1.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(1))

		next, err = sc2.Next()
		require.NoError(t, err)
		require.Equal(t, next, uint64(1))
	})
}
