package shared_testutil

import (
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

func NewTestDealFunds() *TestDealFunds {
	return &TestDealFunds{
		reserved: big.Zero(),
	}
}

type TestDealFunds struct {
	lk           sync.Mutex
	reserved     abi.TokenAmount
	ReserveCalls []abi.TokenAmount
	ReleaseCalls []abi.TokenAmount
}

func (f *TestDealFunds) Get() abi.TokenAmount {
	f.lk.Lock()
	defer f.lk.Unlock()

	return f.reserved
}

func (f *TestDealFunds) Reserve(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.lk.Lock()
	defer f.lk.Unlock()

	f.reserved = big.Add(f.reserved, amount)
	f.ReserveCalls = append(f.ReserveCalls, amount)
	return f.reserved, nil
}

func (f *TestDealFunds) Release(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.lk.Lock()
	defer f.lk.Unlock()

	f.reserved = big.Sub(f.reserved, amount)
	f.ReleaseCalls = append(f.ReleaseCalls, amount)
	return f.reserved, nil
}
