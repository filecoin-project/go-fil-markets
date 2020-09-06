package funds

import (
	"bytes"
	"sync"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

// DealFunds is used to track funds needed for (possibly multiple) deals in progress
type DealFunds interface {
	// returns the current amount tracked
	Get() abi.TokenAmount

	// Reserve is used to mark funds as "in-use" for a deal
	// returns the new amount tracked
	Reserve(amount abi.TokenAmount) (abi.TokenAmount, error)

	// Release releases reserved committed funds back to the available pool
	// returns total amount reserved afterwards
	Release(amount abi.TokenAmount) (abi.TokenAmount, error)
}

type dealFundsImpl struct {
	lock sync.Mutex

	// cached value
	reserved abi.TokenAmount

	key datastore.Key
	ds  datastore.Batching
}

func NewDealFunds(ds datastore.Batching, key datastore.Key) (DealFunds, error) {
	df := &dealFundsImpl{
		ds:  ds,
		key: key,
	}

	value, err := df.loadReserved()
	if err != nil {
		return nil, err
	}

	df.reserved = value

	return df, nil
}

func (f *dealFundsImpl) Get() abi.TokenAmount {
	return f.reserved
}

func (f *dealFundsImpl) Reserve(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.storeReserved(big.Add(f.reserved, amount))
}

func (f *dealFundsImpl) Release(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.storeReserved(big.Sub(f.reserved, amount))
}

// loadReserved will try to load our reserved value from the datastore
// if it cannot find our key, it will return zero
func (f *dealFundsImpl) loadReserved() (abi.TokenAmount, error) {
	b, err := f.ds.Get(f.key)
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			f.reserved = big.Zero()
			return f.reserved, nil
		}
		return abi.TokenAmount{}, err
	}

	var value abi.TokenAmount
	if err = value.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return abi.TokenAmount{}, err
	}

	f.reserved = value
	return f.reserved, nil
}

// stores the new reserved value and returns it
func (f *dealFundsImpl) storeReserved(amount abi.TokenAmount) (abi.TokenAmount, error) {
	var buf bytes.Buffer
	err := amount.MarshalCBOR(&buf)
	if err != nil {
		return abi.TokenAmount{}, err
	}

	if err := f.ds.Put(f.key, buf.Bytes()); err != nil {
		return abi.TokenAmount{}, err
	}

	f.reserved = amount
	return f.reserved, nil
}
