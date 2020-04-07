package storedask_test

import (
	"errors"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestStoredAsk(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	spn := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState: testnodes.NewStorageMarketState(),
		},
	}
	actor := address.TestAddress2
	storedAsk, err := storedask.NewStoredAsk(ds, spn, actor)
	require.NoError(t, err)

	testPrice := abi.NewTokenAmount(1000000000)
	testDuration := abi.ChainEpoch(200)
	t.Run("auto initializing", func(t *testing.T) {
		ask := storedAsk.GetAsk(actor)
		require.NotNil(t, ask)
	})
	t.Run("setting ask price", func(t *testing.T) {
		err := storedAsk.AddAsk(testPrice, testDuration, nil)
		require.NoError(t, err)
		ask := storedAsk.GetAsk(actor)
		require.Equal(t, ask.Ask.Price, testPrice)
		require.Equal(t, ask.Ask.Expiry-ask.Ask.Timestamp, testDuration)
	})
	t.Run("querying incorrect address", func(t *testing.T) {
		otherAddr, err := address.NewActorAddress([]byte("other"))
		require.NoError(t, err)
		ask := storedAsk.GetAsk(otherAddr)
		require.Nil(t, ask)
	})
	t.Run("reloading stored ask from disk", func(t *testing.T) {
		storedAsk2, err := storedask.NewStoredAsk(ds, spn, actor)
		require.NoError(t, err)
		ask := storedAsk2.GetAsk(actor)
		require.Equal(t, ask.Ask.Price, testPrice)
		require.Equal(t, ask.Ask.Expiry-ask.Ask.Timestamp, testDuration)
	})
	t.Run("node errors", func(t *testing.T) {
		spnStateIDErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				GetChainHeadError: errors.New("something went wrong"),
				SMState:           testnodes.NewStorageMarketState(),
			},
		}
		// should load cause ask is is still in data store
		storedAskError, err := storedask.NewStoredAsk(ds, spnStateIDErr, actor)
		require.NoError(t, err)
		err = storedAskError.AddAsk(testPrice, testDuration, nil)
		require.Error(t, err)

		spnMinerWorkerErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
			MinerWorkerError: errors.New("something went wrong"),
		}
		// should load cause ask is is still in data store
		storedAskError, err = storedask.NewStoredAsk(ds, spnMinerWorkerErr, actor)
		require.NoError(t, err)
		err = storedAskError.AddAsk(testPrice, testDuration, nil)
		require.Error(t, err)

		spnSignBytesErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
			SignBytesError: errors.New("something went wrong"),
		}
		// should load cause ask is is still in data store
		storedAskError, err = storedask.NewStoredAsk(ds, spnSignBytesErr, actor)
		require.NoError(t, err)
		err = storedAskError.AddAsk(testPrice, testDuration, nil)
		require.Error(t, err)
	})
}
