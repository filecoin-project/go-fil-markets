package askstore_test

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/askstore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/shared"
)

func TestAskStoreImpl(t *testing.T) {
	minerWorker := tutils.NewActorAddr(t, "minerworker")
	api := &mockAskStoreAPI{minerWorker: minerWorker}
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	store, err := askstore.NewAskStore(ds, datastore.NewKey("retrieval-ask"), api, minerWorker)
	require.NoError(t, err)

	// A new store returns the default ask
	ask := store.GetAsk()
	require.NotNil(t, ask)

	require.Equal(t, retrievalmarket.DefaultUnsealPrice, ask.UnsealPrice)
	require.Equal(t, retrievalmarket.DefaultPricePerByte, ask.PricePerByte)
	require.Equal(t, retrievalmarket.DefaultPaymentInterval, ask.PaymentInterval)
	require.Equal(t, retrievalmarket.DefaultPaymentIntervalIncrease, ask.PaymentIntervalIncrease)

	// Fetch signed ask
	signedAsk, err := store.GetSignedAsk()
	require.NoError(t, err)
	require.NotNil(t, signedAsk.Signature)

	// Store a new ask
	newAsk := &retrievalmarket.Ask{
		PricePerByte:            abi.NewTokenAmount(123),
		UnsealPrice:             abi.NewTokenAmount(456),
		PaymentInterval:         789,
		PaymentIntervalIncrease: 789,
	}
	err = store.SetAsk(newAsk)
	require.NoError(t, err)

	// Fetch new ask
	stored := store.GetAsk()
	require.Equal(t, newAsk, stored)

	// Fetch signed new ask
	signedNewAsk, err := store.GetSignedAsk()
	require.NoError(t, err)
	require.Equal(t, newAsk, signedNewAsk.Ask)
	require.NotNil(t, signedNewAsk.Signature)
	require.NotEqual(t, signedAsk.Signature, signedNewAsk.Signature)

	// Construct a new AskStore and make sure it returns the previously-stored ask
	newStore, err := askstore.NewAskStore(ds, datastore.NewKey("retrieval-ask"), api, minerWorker)
	require.NoError(t, err)
	stored = newStore.GetAsk()
	require.Equal(t, newAsk, stored)
	storedSignedNewAsk, err := store.GetSignedAsk()
	require.NoError(t, err)
	require.Equal(t, newAsk, storedSignedNewAsk.Ask)
	require.NotNil(t, storedSignedNewAsk.Signature)
	require.Equal(t, storedSignedNewAsk.Signature, signedNewAsk.Signature)
}

func TestMigrations(t *testing.T) {
	minerWorker := tutils.NewActorAddr(t, "minerworker")
	api := &mockAskStoreAPI{minerWorker: minerWorker}
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	oldAsk := &migrations.Ask0{
		PricePerByte:            abi.NewTokenAmount(rand.Int63()),
		UnsealPrice:             abi.NewTokenAmount(rand.Int63()),
		PaymentInterval:         rand.Uint64(),
		PaymentIntervalIncrease: rand.Uint64(),
	}
	buf := new(bytes.Buffer)
	err := oldAsk.MarshalCBOR(buf)
	require.NoError(t, err)
	ds.Put(datastore.NewKey("retrieval-ask"), buf.Bytes())
	newStore, err := askstore.NewAskStore(ds, datastore.NewKey("retrieval-ask"), api, minerWorker)
	require.NoError(t, err)
	ask := newStore.GetAsk()
	expectedAsk := &retrievalmarket.Ask{
		PricePerByte:            oldAsk.PricePerByte,
		UnsealPrice:             oldAsk.UnsealPrice,
		PaymentInterval:         oldAsk.PaymentInterval,
		PaymentIntervalIncrease: oldAsk.PaymentIntervalIncrease,
	}
	require.Equal(t, expectedAsk, ask)
}

type mockAskStoreAPI struct {
	minerWorker address.Address
}

var _ askstore.AskStoreAPI = (*mockAskStoreAPI)(nil)

func (m *mockAskStoreAPI) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	return nil, abi.ChainEpoch(0), nil
}

func (m *mockAskStoreAPI) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	return m.minerWorker, nil
}

func (m *mockAskStoreAPI) SignBytes(ctx context.Context, address address.Address, i []byte) (*crypto.Signature, error) {
	return &crypto.Signature{
		Type: crypto.SigTypeSecp256k1,
		Data: append([]byte("sig"), i...),
	}, nil
}
