package providerstates_test

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
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	testnet "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestReceiveDeal(t *testing.T) {
	ctx := context.Background()

	environment := func(node retrievalmarket.RetrievalProviderNode, params testnet.TestDealStreamParams) *testProviderDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(params)
		return NewTestProviderDealEnvironment(node, ds, nil)
	}

	blankDealState := func() *retrievalmarket.ProviderDealState {
		return &retrievalmarket.ProviderDealState{
			Status:        retrievalmarket.DealStatusNew,
			TotalSent:     0,
			FundsReceived: tokenamount.FromInt(0),
		}
	}

	expectedPiece := []byte(string("applesauce"))
	proposal := retrievalmarket.DealProposal{
		ID:       retrievalmarket.DealID(10),
		PieceCID: expectedPiece,
		Params: retrievalmarket.Params{
			PricePerByte:            defaultPricePerByte,
			PaymentInterval:         defaultCurrentInterval,
			PaymentIntervalIncrease: defaultIntervalIncrease,
		},
	}

	t.Run("it works", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectPiece(expectedPiece, 10000)
		dealState := blankDealState()
		expectedDealResponse := retrievalmarket.DealResponse{
			Status: retrievalmarket.DealStatusAccepted,
			ID:     proposal.ID,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			ProposalReader: testnet.StubbedDealProposalReader(proposal),
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		})
		fe.ExpectParams(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease, nil)
		f := providerstates.ReceiveDeal(ctx, fe, *dealState)
		fe.VerifyExpectations(t)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusAccepted)
		require.Equal(t, dealState.DealProposal, proposal)
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval)
		require.Empty(t, dealState.Message)
	})

	t.Run("missing piece", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectMissingPiece(expectedPiece)
		dealState := blankDealState()
		expectedDealResponse := retrievalmarket.DealResponse{
			Status:  retrievalmarket.DealStatusDealNotFound,
			ID:      proposal.ID,
			Message: retrievalmarket.ErrNotFound.Error(),
		}
		fe := environment(node, testnet.TestDealStreamParams{
			ProposalReader: testnet.StubbedDealProposalReader(proposal),
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		})
		f := providerstates.ReceiveDeal(ctx, fe, *dealState)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusDealNotFound)
		require.NotEmpty(t, dealState.Message)
	})

	t.Run("deal rejected", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectPiece(expectedPiece, 10000)
		dealState := blankDealState()
		message := "Something Terrible Happened"
		expectedDealResponse := retrievalmarket.DealResponse{
			Status:  retrievalmarket.DealStatusRejected,
			ID:      proposal.ID,
			Message: message,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			ProposalReader: testnet.StubbedDealProposalReader(proposal),
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		})
		fe.ExpectParams(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease, errors.New(message))
		f := providerstates.ReceiveDeal(ctx, fe, *dealState)
		fe.VerifyExpectations(t)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusRejected)
		require.NotEmpty(t, dealState.Message)
	})

	t.Run("proposal read error", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := blankDealState()
		fe := environment(node, testnet.TestDealStreamParams{
			ProposalReader: testnet.FailDealProposalReader,
		})
		f := providerstates.ReceiveDeal(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})

	t.Run("response write error", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectPiece(expectedPiece, 10000)
		dealState := blankDealState()
		fe := environment(node, testnet.TestDealStreamParams{
			ProposalReader: testnet.StubbedDealProposalReader(proposal),
			ResponseWriter: testnet.FailDealResponseWriter,
		})
		fe.ExpectParams(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease, nil)
		f := providerstates.ReceiveDeal(ctx, fe, *dealState)
		fe.VerifyExpectations(t)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})

}

func TestSendBlocks(t *testing.T) {
	ctx := context.Background()
	node := testnodes.NewTestRetrievalProviderNode()

	environment := func(params testnet.TestDealStreamParams, responses []readBlockResponse) *testProviderDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(params)
		return NewTestProviderDealEnvironment(node, ds, responses)
	}

	t.Run("it works", func(t *testing.T) {
		blocks, responses := generateResponses(10, 100, false, false)
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
		expectedDealResponse := retrievalmarket.DealResponse{
			Status:      retrievalmarket.DealStatusFundsNeeded,
			PaymentOwed: defaultPaymentPerInterval,
			Blocks:      blocks,
			ID:          dealState.ID,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		}, responses)
		f := providerstates.SendBlocks(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFundsNeeded)
		require.Equal(t, dealState.TotalSent, defaultTotalSent+defaultCurrentInterval)
		require.Empty(t, dealState.Message)
	})

	t.Run("it completes", func(t *testing.T) {
		blocks, responses := generateResponses(10, 100, true, false)
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
		expectedDealResponse := retrievalmarket.DealResponse{
			Status:      retrievalmarket.DealStatusFundsNeededLastPayment,
			PaymentOwed: defaultPaymentPerInterval,
			Blocks:      blocks,
			ID:          dealState.ID,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		}, responses)
		f := providerstates.SendBlocks(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFundsNeededLastPayment)
		require.Equal(t, dealState.TotalSent, defaultTotalSent+defaultCurrentInterval)
		require.Empty(t, dealState.Message)
	})

	t.Run("error reading a block", func(t *testing.T) {
		_, responses := generateResponses(10, 100, false, true)
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
		expectedDealResponse := retrievalmarket.DealResponse{
			Status:  retrievalmarket.DealStatusFailed,
			Message: responses[0].err.Error(),
			ID:      dealState.ID,
		}
		fe := environment(testnet.TestDealStreamParams{
			ResponseWriter: testnet.ExpectDealResponseWriter(t, expectedDealResponse),
		}, responses)
		f := providerstates.SendBlocks(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})

	t.Run("error writing response", func(t *testing.T) {
		_, responses := generateResponses(10, 100, false, false)
		dealState := makeDealState(retrievalmarket.DealStatusAccepted)
		fe := environment(testnet.TestDealStreamParams{
			ResponseWriter: testnet.FailDealResponseWriter,
		}, responses)
		f := providerstates.SendBlocks(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})
}

func TestProcessPayment(t *testing.T) {
	ctx := context.Background()

	environment := func(node retrievalmarket.RetrievalProviderNode, params testnet.TestDealStreamParams) *testProviderDealEnvironment {
		ds := testnet.NewTestRetrievalDealStream(params)
		return NewTestProviderDealEnvironment(node, ds, nil)
	}

	payCh := address.TestAddress
	voucher := testnet.MakeTestSignedVoucher()
	t.Run("it works", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		err := node.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
		require.NoError(t, err)
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.TotalSent = defaultTotalSent + defaultCurrentInterval
		dealPayment := retrievalmarket.DealPayment{
			ID:             dealState.ID,
			PaymentChannel: payCh,
			PaymentVoucher: voucher,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			PaymentReader: testnet.StubbedDealPaymentReader(dealPayment),
		})
		f := providerstates.ProcessPayment(ctx, fe, *dealState)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusOngoing)
		require.Equal(t, dealState.FundsReceived, tokenamount.Add(defaultFundsReceived, defaultPaymentPerInterval))
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval+defaultIntervalIncrease)
		require.Empty(t, dealState.Message)
	})
	t.Run("it completes", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		err := node.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
		require.NoError(t, err)
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeededLastPayment)
		dealState.TotalSent = defaultTotalSent + defaultCurrentInterval
		dealPayment := retrievalmarket.DealPayment{
			ID:             dealState.ID,
			PaymentChannel: payCh,
			PaymentVoucher: voucher,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			PaymentReader: testnet.StubbedDealPaymentReader(dealPayment),
		})
		f := providerstates.ProcessPayment(ctx, fe, *dealState)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusCompleted)
		require.Equal(t, dealState.FundsReceived, tokenamount.Add(defaultFundsReceived, defaultPaymentPerInterval))
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval+defaultIntervalIncrease)
		require.Empty(t, dealState.Message)
	})

	t.Run("not enough funds sent", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		smallerPayment := tokenamount.FromInt(400000)
		err := node.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, smallerPayment, nil)
		require.NoError(t, err)
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.TotalSent = defaultTotalSent + defaultCurrentInterval
		dealPayment := retrievalmarket.DealPayment{
			ID:             dealState.ID,
			PaymentChannel: payCh,
			PaymentVoucher: voucher,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			PaymentReader: testnet.StubbedDealPaymentReader(dealPayment),
			ResponseWriter: testnet.ExpectDealResponseWriter(t, rm.DealResponse{
				ID:          dealState.ID,
				Status:      retrievalmarket.DealStatusFundsNeeded,
				PaymentOwed: tokenamount.Sub(defaultPaymentPerInterval, smallerPayment),
			}),
		})
		f := providerstates.ProcessPayment(ctx, fe, *dealState)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFundsNeeded)
		require.Equal(t, dealState.FundsReceived, tokenamount.Add(defaultFundsReceived, smallerPayment))
		require.Equal(t, dealState.CurrentInterval, defaultCurrentInterval)
		require.Empty(t, dealState.Message)
	})

	t.Run("failure processing payment", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		message := "your money's no good here"
		err := node.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, tokenamount.FromInt(0), errors.New(message))
		require.NoError(t, err)
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.TotalSent = defaultTotalSent + defaultCurrentInterval
		dealPayment := retrievalmarket.DealPayment{
			ID:             dealState.ID,
			PaymentChannel: payCh,
			PaymentVoucher: voucher,
		}
		fe := environment(node, testnet.TestDealStreamParams{
			PaymentReader: testnet.StubbedDealPaymentReader(dealPayment),
			ResponseWriter: testnet.ExpectDealResponseWriter(t, rm.DealResponse{
				ID:      dealState.ID,
				Status:  retrievalmarket.DealStatusFailed,
				Message: message,
			}),
		})
		f := providerstates.ProcessPayment(ctx, fe, *dealState)
		node.VerifyExpectations(t)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})

	t.Run("failure reading payment", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := makeDealState(retrievalmarket.DealStatusFundsNeeded)
		dealState.TotalSent = defaultTotalSent + defaultCurrentInterval
		fe := environment(node, testnet.TestDealStreamParams{
			PaymentReader: testnet.FailDealPaymentReader,
		})
		f := providerstates.ProcessPayment(ctx, fe, *dealState)
		f(dealState)
		require.Equal(t, dealState.Status, retrievalmarket.DealStatusFailed)
		require.NotEmpty(t, dealState.Message)
	})
}

type readBlockResponse struct {
	block retrievalmarket.Block
	done  bool
	err   error
}

type dealParamsKey struct {
	pricePerByte            string
	paymentInterval         uint64
	paymentIntervalIncrease uint64
}

type testProviderDealEnvironment struct {
	node           retrievalmarket.RetrievalProviderNode
	ds             rmnet.RetrievalDealStream
	nextResponse   int
	responses      []readBlockResponse
	expectedParams map[dealParamsKey]error
	receivedParams map[dealParamsKey]struct{}
}

func NewTestProviderDealEnvironment(node retrievalmarket.RetrievalProviderNode,
	ds rmnet.RetrievalDealStream,
	responses []readBlockResponse) *testProviderDealEnvironment {
	return &testProviderDealEnvironment{node, ds, 0, responses, make(map[dealParamsKey]error), make(map[dealParamsKey]struct{})}
}

func (te *testProviderDealEnvironment) ExpectParams(pricePerByte tokenamount.TokenAmount,
	paymentInterval uint64,
	paymentIntervalIncrease uint64,
	response error) {
	te.expectedParams[dealParamsKey{pricePerByte.String(), paymentInterval, paymentIntervalIncrease}] = response
}

func (te *testProviderDealEnvironment) VerifyExpectations(t *testing.T) {
	require.Equal(t, len(te.expectedParams), len(te.receivedParams))
}

func (te *testProviderDealEnvironment) Node() rm.RetrievalProviderNode {
	return te.node
}

func (te *testProviderDealEnvironment) DealStream() rmnet.RetrievalDealStream {
	return te.ds
}

func (te *testProviderDealEnvironment) CheckDealParams(pricePerByte tokenamount.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) error {
	key := dealParamsKey{pricePerByte.String(), paymentInterval, paymentIntervalIncrease}
	err, ok := te.expectedParams[key]
	if !ok {
		return errors.New("Something went wrong")
	}
	te.receivedParams[key] = struct{}{}
	return err
}

func (te *testProviderDealEnvironment) NextBlock(_ context.Context) (rm.Block, bool, error) {
	if te.nextResponse >= len(te.responses) {
		return rm.EmptyBlock, false, errors.New("Something went wrong")
	}
	response := te.responses[te.nextResponse]
	te.nextResponse += 1
	return response.block, response.done, response.err
}

var defaultCurrentInterval = uint64(1000)
var defaultIntervalIncrease = uint64(500)
var defaultPricePerByte = tokenamount.FromInt(500)
var defaultPaymentPerInterval = tokenamount.Mul(defaultPricePerByte, tokenamount.FromInt(defaultCurrentInterval))
var defaultTotalSent = uint64(5000)
var defaultFundsReceived = tokenamount.FromInt(2500000)

func makeDealState(status retrievalmarket.DealStatus) *retrievalmarket.ProviderDealState {
	return &retrievalmarket.ProviderDealState{
		Status:          status,
		TotalSent:       defaultTotalSent,
		CurrentInterval: defaultCurrentInterval,
		FundsReceived:   defaultFundsReceived,
		DealProposal: retrievalmarket.DealProposal{
			ID: retrievalmarket.DealID(10),
			Params: retrievalmarket.Params{
				PricePerByte:            defaultPricePerByte,
				PaymentInterval:         defaultCurrentInterval,
				PaymentIntervalIncrease: defaultIntervalIncrease,
			},
		},
	}
}

func generateResponses(count uint64, blockSize uint64, completeOnLast bool, errorOnFirst bool) ([]retrievalmarket.Block, []readBlockResponse) {
	responses := make([]readBlockResponse, count)
	blocks := make([]retrievalmarket.Block, count)
	var i uint64 = 0
	for ; i < count; i++ {
		data := make([]byte, blockSize)
		var err error
		_, err = rand.Read(data)
		complete := false
		if i == 0 && errorOnFirst {
			err = errors.New("something went wrong")
		}

		if i == count-1 && completeOnLast {
			complete = true
		}
		block := retrievalmarket.Block{
			Prefix: cid.NewPrefixV1(cid.Raw, mh.SHA2_256).Bytes(),
			Data:   data,
		}
		blocks[i] = block
		responses[i] = readBlockResponse{
			block, complete, err}
	}
	return blocks, responses
}
