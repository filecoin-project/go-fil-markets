package funds_test

import (
	"testing"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
)

func TestDealFunds(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	key := datastore.NewKey("deal_funds_test")

	f, err := funds.NewDealFunds(ds, key)
	assert.NoError(t, err)

	// initializes to zero
	assert.Equal(t, f.Get(), big.Zero())

	// reserve funds and return new total
	newAmount, err := f.Reserve(abi.NewTokenAmount(123))
	assert.NoError(t, err)
	assert.Equal(t, abi.NewTokenAmount(123), newAmount)
	assert.Equal(t, abi.NewTokenAmount(123), f.Get())

	// reserve more funds and return new total
	newAmount, err = f.Reserve(abi.NewTokenAmount(100))
	assert.NoError(t, err)
	assert.Equal(t, abi.NewTokenAmount(223), newAmount)
	assert.Equal(t, abi.NewTokenAmount(223), f.Get())

	// release funds and return new total
	newAmount, err = f.Release(abi.NewTokenAmount(123))
	assert.NoError(t, err)
	assert.Equal(t, abi.NewTokenAmount(100), newAmount)
	assert.Equal(t, abi.NewTokenAmount(100), f.Get())

	// creating new funds will read stored value
	f, err = funds.NewDealFunds(ds, key)
	assert.NoError(t, err)
	assert.Equal(t, abi.NewTokenAmount(100), newAmount)
	assert.Equal(t, abi.NewTokenAmount(100), f.Get())
}
