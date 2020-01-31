package clientstates_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	clientstates "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	testnet "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

type consumeBlockResponse struct {
	size uint64
	done bool
	err  error
}

type fakeEnvironment struct {
	node         retrievalmarket.RetrievalClientNode
	ds           rmnet.RetrievalDealStream
	nextResponse int
	responses    []consumeBlockResponse
}

func (e *fakeEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return e.node
}

func (e *fakeEnvironment) DealStream() rmnet.RetrievalDealStream {
	return e.ds
}

func (e *fakeEnvironment) ConsumeBlock(context.Context, retrievalmarket.Block) (uint64, bool, error) {
	if e.nextResponse >= len(e.responses) {
		return 0, false, errors.New("ConsumeBlock failed")
	}
	response := e.responses[e.nextResponse]
	e.nextResponse += 1
	return response.size, response.done, response.err
}

func TestSetupPaymentChannel(t *testing.T) {
	ctx := context.Background()
	ds := testnet.NewTestRetrievalDealStream(testnet.TestDealStreamParams{})
	expectedPayCh := address.TestAddress2
	expectedLane := uint64(10)

	environment := func(params testnodes.TestRetrievalClientNodeParams) clientstates.ClientDealEnvironment {
		node := testnodes.NewTestRetrievalClientNode(params)
		return &fakeEnvironment{node, ds, 0, nil}
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)

		fe := environment(testnodes.TestRetrievalClientNodeParams{
			PayCh: expectedPayCh,
			Lane:  expectedLane,
		})
		f := clientstates.SetupPaymentChannel(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusPaymentChannelCreated)
		require.Equal(t, dealState.PayCh, expectedPayCh)
		require.Equal(t, dealState.Lane, expectedLane)
	})

	t.Run("when create payment channel fails", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
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
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
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
	node := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{})

	environment := func(params testnet.TestDealStreamParams) clientstates.ClientDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(params)
		return &fakeEnvironment{node, ds, 0, nil}
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusNew)

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
		dealState := makeDealState(retrievalmarket.DealStatusNew)
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

	t.Run("deal not found", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusNew)
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(retrievalmarket.DealResponse{
				Status:  retrievalmarket.DealStatusDealNotFound,
				ID:      dealState.ID,
				Message: "can't find a deal",
			}),
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusDealNotFound)
	})

	t.Run("unable to send proposal", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusNew)
		fe := environment(testnet.TestDealStreamParams{
			ProposalWriter: testnet.FailDealProposalWriter,
		})
		f := clientstates.ProposeDeal(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("unable to read response", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusNew)
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

	environment := func(netParams testnet.TestDealStreamParams,
		nodeParams testnodes.TestRetrievalClientNodeParams) clientstates.ClientDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(netParams)
		node := testnodes.NewTestRetrievalClientNode(nodeParams)
		return &fakeEnvironment{node, ds, 0, nil}
	}

	testVoucher := &types.SignedVoucher{}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.Add(defaultFundsSpent, defaultPaymentRequested))
		require.Equal(t, dealState.BytesPaidFor, defaultTotalReceived)
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval+defaultIntervalIncrease)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("last payment", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeededLastPayment)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.Add(defaultFundsSpent, defaultPaymentRequested))
		require.Equal(t, dealState.BytesPaidFor, defaultTotalReceived)
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval+defaultIntervalIncrease)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusCompleted)
	})

	t.Run("not enough funds left", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.FundsSpent = defaultTotalFunds
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("not enough bytes since last payment", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.BytesPaidFor = defaultBytesPaidFor + 500
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("more bytes since last payment than interval works, can charge more", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.BytesPaidFor = defaultBytesPaidFor - 500
		largerPaymentRequested := tokenamount.FromInt(750000)
		dealState.PaymentRequested = largerPaymentRequested

		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.Add(defaultFundsSpent, largerPaymentRequested))
		require.Equal(t, dealState.BytesPaidFor, defaultTotalReceived)
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval+defaultIntervalIncrease)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("too much payment requested", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.PaymentRequested = tokenamount.FromInt(750000)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("too little payment requested works but records correctly", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		smallerPaymentRequested := tokenamount.FromInt(250000)
		dealState.PaymentRequested = smallerPaymentRequested
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			Voucher: testVoucher,
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.PaymentRequested, tokenamount.FromInt(0))
		require.Equal(t, dealState.FundsSpent, tokenamount.Add(defaultFundsSpent, smallerPaymentRequested))
		// only records change for those bytes paid for
		require.Equal(t, dealState.BytesPaidFor, defaultBytesPaidFor+500)
		// no interval increase
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("voucher create fails", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		fe := environment(testnet.TestDealStreamParams{}, testnodes.TestRetrievalClientNodeParams{
			VoucherError: errors.New("Something Went Wrong"),
		})
		f := clientstates.ProcessPaymentRequested(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("unable to send payment", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
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

func TestProcessNextResponse(t *testing.T) {
	ctx := context.Background()
	node := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{})

	environment := func(netParams testnet.TestDealStreamParams,
		responses []consumeBlockResponse) clientstates.ClientDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(netParams)
		return &fakeEnvironment{node, ds, 0, responses}
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, false, false)
		response := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusOngoing,
			ID:     dealState.ID,
			Blocks: blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived+1000)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
	})

	t.Run("completes", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, true, false)
		response := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusCompleted,
			ID:     dealState.ID,
			Blocks: blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived+1000)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusCompleted)
	})

	t.Run("completes last payment", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, true, false)
		response := retrievalmarket.DealResponse{
			Status:      retrievalmarket.DealStatusFundsNeededLastPayment,
			ID:          dealState.ID,
			PaymentOwed: tokenamount.FromInt(1000),
			Blocks:      blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived+1000)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFundsNeededLastPayment)
		require.Equal(t, dealState.PaymentRequested, response.PaymentOwed)
	})

	t.Run("receive complete status but deal is not complete errors", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, false, false)
		response := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusCompleted,
			ID:     dealState.ID,
			Blocks: blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})
	t.Run("payment requested", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, false, false)
		response := retrievalmarket.DealResponse{
			Status:      retrievalmarket.DealStatusFundsNeeded,
			ID:          dealState.ID,
			PaymentOwed: tokenamount.FromInt(1000),
			Blocks:      blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.Empty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived+1000)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFundsNeeded)
		require.Equal(t, dealState.PaymentRequested, response.PaymentOwed)
	})

	t.Run("unexpected status errors", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, false, false)
		response := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusNew,
			ID:     dealState.ID,
			Blocks: blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("consume block errors", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		blocks, consumeBlockResponses := generateBlocks(10, 100, false, true)
		response := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusOngoing,
			ID:     dealState.ID,
			Blocks: blocks,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.StubbedDealResponseReader(response),
		}, consumeBlockResponses)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})

	t.Run("read response errors", func(t *testing.T) {
		dealState := makeDealState(retrievalmarket.DealStatusOngoing)
		fe := environment(testnet.TestDealStreamParams{
			ResponseReader: testnet.FailDealResponseReader,
		}, nil)
		f := clientstates.ProcessNextResponse(ctx, fe, *dealState)
		f(dealState)
		require.NotEmpty(t, dealState.Message)
		require.Equal(t, dealState.TotalReceived, defaultTotalReceived)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
	})
}

var defaultTotalFunds = tokenamount.FromInt(4000000)
var defaultCurrentInterval = uint64(1000)
var defaultIntervalIncrease = uint64(500)
var defaultPricePerByte = tokenamount.FromInt(500)
var defaultTotalReceived = uint64(6000)
var defaultBytesPaidFor = uint64(5000)
var defaultFundsSpent = tokenamount.FromInt(2500000)
var defaultPaymentRequested = tokenamount.FromInt(500000)

func makeDealState(status retrievalmarket.DealStatus) *retrievalmarket.ClientDealState {
	return &retrievalmarket.ClientDealState{
		TotalFunds:       defaultTotalFunds,
		MinerWallet:      address.TestAddress,
		ClientWallet:     address.TestAddress2,
		PayCh:            address.TestAddress2,
		Lane:             uint64(10),
		Status:           status,
		BytesPaidFor:     defaultBytesPaidFor,
		TotalReceived:    defaultTotalReceived,
		CurrentInterval:  defaultCurrentInterval,
		FundsSpent:       defaultFundsSpent,
		PaymentRequested: defaultPaymentRequested,
		DealProposal: retrievalmarket.DealProposal{
			ID: retrievalmarket.DealID(10),
			Params: retrievalmarket.Params{
				PricePerByte:            defaultPricePerByte,
				PaymentIntervalIncrease: defaultIntervalIncrease,
			},
		},
	}
}

func generateBlocks(count uint64, blockSize uint64, completeOnLast bool, errorOnFirst bool) ([]retrievalmarket.Block, []consumeBlockResponse) {
	blocks := make([]retrievalmarket.Block, count)
	responses := make([]consumeBlockResponse, count)
	var i uint64 = 0
	for ; i < count; i++ {
		data := make([]byte, blockSize)
		var err error
		_, err = rand.Read(data)
		blocks[i] = retrievalmarket.Block{
			Prefix: cid.NewPrefixV1(cid.Raw, mh.SHA2_256).Bytes(),
			Data:   data,
		}
		complete := false
		if i == 0 && errorOnFirst {
			err = errors.New("something went wrong")
		}

		if i == count-1 && completeOnLast {
			complete = true
		}
		responses[i] = consumeBlockResponse{blockSize, complete, err}
	}
	return blocks, responses
}
