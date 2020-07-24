package shared_testutil

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
)

func NewTestDealFunds() *TestDealFunds {
	return &TestDealFunds{
		reserved: big.Zero(),
	}
}

type TestDealFunds struct {
	reserved     abi.TokenAmount
	ReserveCalls []abi.TokenAmount
	ReleaseCalls []abi.TokenAmount
}

func (f *TestDealFunds) Reserve(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.reserved = big.Add(f.reserved, amount)
	f.ReserveCalls = append(f.ReserveCalls, amount)
	return f.reserved, nil
}

func (f *TestDealFunds) Release(amount abi.TokenAmount) (abi.TokenAmount, error) {
	f.reserved = big.Sub(f.reserved, amount)
	f.ReleaseCalls = append(f.ReleaseCalls, amount)
	return f.reserved, nil
}

var _ funds.DealFunds = &TestDealFunds{}
