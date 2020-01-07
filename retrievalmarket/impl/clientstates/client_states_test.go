package clientstates_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	clientstates "github.com/filecoin-project/go-fil-components/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-components/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-components/retrievalmarket/network"
	testnet "github.com/filecoin-project/go-fil-components/retrievalmarket/network/testutil"
	"github.com/filecoin-project/go-fil-components/shared/tokenamount"
	"github.com/filecoin-project/go-fil-components/shared/types"
)

type fakeEnvironment struct {
	node retrievalmarket.RetrievalClientNode
	ds   rmnet.RetrievalDealStream
	bs   bstore.Blockstore
}

func (e fakeEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return e.node
}

func (e fakeEnvironment) DealStream() rmnet.RetrievalDealStream {
	return e.ds
}

func (e fakeEnvironment) Blockstore() blockstore.Blockstore {
	return e.bs
}

func TestSetupPaymentChannel(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	ds := testnet.NewTestRetrievalDealStream(testnet.TestDealStreamParams{})
	expectedPayCh := address.TestAddress2
	expectedLane := uint64(10)

	blankDealState := func() *retrievalmarket.ClientDealState {
		return &retrievalmarket.ClientDealState{
			TotalFunds:   tokenamount.FromInt(1234),
			MinerWallet:  address.TestAddress,
			ClientWallet: address.TestAddress2,
		}
	}

	environment := func(params testnodes.TestRetrievalClientNodeParams) clientstates.ClientDealEnvironment {
		node := testnodes.NewTestRetrievalClientNode(params)
		return &fakeEnvironment{node, ds, bs}
	}

	t.Run("it works", func(t *testing.T) {
		dealState := blankDealState()

		fe := environment(testnodes.TestRetrievalClientNodeParams{
			PayCh: expectedPayCh,
			Lane:  expectedLane,
		})
		f := clientstates.SetupPaymentChannel(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusPaymentChannelCreated)
		require.Equal(t, dealState.PayCh, expectedPayCh)
		require.Equal(t, dealState.Lane, expectedLane)
	})

	t.Run("when create payment channel fails", func(t *testing.T) {
		dealState := blankDealState()

		fe := environment(testnodes.TestRetrievalClientNodeParams{
			PayCh:    address.Undef,
			PayChErr: errors.New("Something went wrong"),
			Lane:     expectedLane,
		})
		f := clientstates.SetupPaymentChannel(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("when allocate lane fails", func(t *testing.T) {
		dealState := blankDealState()

		fe := environment(testnodes.TestRetrievalClientNodeParams{
			PayCh:     expectedPayCh,
			Lane:      expectedLane,
			LaneError: errors.New("Something went wrong"),
		})

		f := clientstates.SetupPaymentChannel(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})
}

func TestProposeDeal(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	node := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{})

	blankDealState := func() *retrievalmarket.ClientDealState {
		return &retrievalmarket.ClientDealState{
			TotalFunds:   tokenamount.FromInt(1234),
			MinerWallet:  address.TestAddress,
			ClientWallet: address.TestAddress2,
			PayCh:        address.TestAddress2,
			Lane:         uint64(10),
			DealProposal: retrievalmarket.DealProposal{
				ID: retrievalmarket.DealID(10),
			},
		}
	}

	environment := func(params testnet.TestDealStreamParams) clientstates.ClientDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(params)
		return &fakeEnvironment{node, ds, bs}
	}

	t.Run("it works", func(t *testing.T) {
		dealState := blankDealState()
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(retrievalmarket.DealResponse{
				Status: retrievalmarket.DealStatusAccepted,
				ID:     dealState.ID,
			}),
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusAccepted)
	})

	t.Run("deal rejected", func(t *testing.T) {
		dealState := blankDealState()
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(retrievalmarket.DealResponse{
				Status:  retrievalmarket.DealStatusRejected,
				ID:      dealState.ID,
				Message: "your deal proposal sucks",
			}),
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusRejected)
	})

	t.Run("unable to send proposal", func(t *testing.T) {
		dealState := blankDealState()
		fe := environment(testnet.TestDealStreamParams{
			ProposalWriter: testnet.FailDealProposalWriter,
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("unable to read response", func(t *testing.T) {
		dealState := blankDealState()
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.FailDealResponseReader,
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})
}

func TestProcessPaymentRequested(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	type dealStateParams struct {
		totalFunds       tokenamount.TokenAmount
		currentInterval  uint64
		intervalIncrease uint64
		pricePerByte     tokenamount.TokenAmount
		totalReceived    uint64
		bytesPaidFor     uint64
		fundsSpent       tokenamount.TokenAmount
		paymentRequested tokenamount.TokenAmount
	}

	defaultTotalFunds := tokenamount.FromInt(4000000)
	defaultCurrentInterval := uint64(1000)
	defaultIntervalIncrease := uint64(500)
	defaultPricePerByte := tokenamount.FromInt(500)
	defaultTotalReceived := uint64(6000)
	defaultBytesPaidFor := uint64(5000)
	defaultFundsSpent := tokenamount.FromInt(2500000)
	defaultPaymentRequested := tokenamount.FromInt(50000)
	defaultParams := func() dealStateParams {
		return dealStateParams{
			defaultTotalFunds, defaultCurrentInterval, defaultIntervalIncrease,
			defaultPricePerByte, defaultTotalReceived, defaultBytesPaidFor, defaultFundsSpent,
			defaultPaymentRequested,
		}
	}

	makeDealState := func(params dealStateParams) *retrievalmarket.ClientDealState {
		return &retrievalmarket.ClientDealState{
			TotalFunds:       params.totalFunds,
			MinerWallet:      address.TestAddress,
			ClientWallet:     address.TestAddress2,
			PayCh:            address.TestAddress2,
			Lane:             uint64(10),
			Status:           retrievalmarket.DealStatusFundsNeeded,
			BytesPaidFor:     params.bytesPaidFor,
			TotalReceived:    params.totalReceived,
			CurrentInterval:  params.currentInterval,
			FundsSpent:       params.fundsSpent,
			PaymentRequested: params.paymentRequested,
			DealProposal: retrievalmarket.DealProposal{
				ID: retrievalmarket.DealID(10),
				Params: retrievalmarket.Params{
					PricePerByte:            params.pricePerByte,
					PaymentIntervalIncrease: params.intervalIncrease,
				},
			},
		}
	}

	environment := func(netParams testnet.TestDealStreamParams,
		nodeParams testnodes.TestRetrievalClientNodeParams) clientstates.ClientDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(netParams)
		node := testnodes.NewTestRetrievalClientNode(nodeParams)
		return &fakeEnvironment{node, ds, bs}
	}

	testVoucher := &types.SignedVoucher{}

	t.Run("it works", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.FromInt(3000000))
		require.Equal(t, dealState.BytesPaidFor, 6000)
		require.Equal(t, dealState.CurrentInterval, 1500)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("not enough funds left", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealStateParams.fundsSpent = defaultTotalFunds
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("not enough bytes since last payment", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealStateParams.bytesPaidFor = defaultBytesPaidFor + 500
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("more bytes since last payment than interval works, can charge more", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealStateParams.bytesPaidFor = defaultBytesPaidFor - 500
		dealStateParams.paymentRequested = tokenamount.FromInt(750000)
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.FromInt(3250000))
		require.Equal(t, dealState.BytesPaidFor, 6000)
		require.Equal(t, dealState.CurrentInterval, 1500)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("too much payment requested", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealStateParams.paymentRequested = tokenamount.FromInt(750000)
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("too little payment requested works but records correctly", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealStateParams.paymentRequested = tokenamount.FromInt(250000)
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.FromInt(2750000))
		require.Equal(t, dealState.BytesPaidFor, 5500)
		require.Equal(t, dealState.CurrentInterval, 1000)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("voucher create fails", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			VoucherError: errors.New("Something Went Wrong"),
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("unable to send payment", func(t *testing.T) {
		dealStateParams := defaultParams()
		dealState := makeDealState(dealStateParams)
		fe := environment(testnet.TestDealStreamParams{
			PaymentWriter: testnet.FailDealPaymentWriter,
		}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})
}
