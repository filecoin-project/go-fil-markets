package requestvalidation_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestOnPushDataReceived(t *testing.T) {
	fre := &fakeRevalidatorEnvironment{}
	revalidator := requestvalidation.NewProviderRevalidator(fre)
	channelID := shared_testutil.MakeTestChannelID()
	voucherResult, err := revalidator.OnPushDataReceived(channelID, rand.Uint64())
	require.NoError(t, err)
	require.Nil(t, voucherResult)
}
func TestOnPullDataSent(t *testing.T) {

	deal := *makeDealState(rm.DealStatusOngoing)
	testCases := map[string]struct {
		noSend         bool
		expectedID     rm.ProviderDealIdentifier
		expectedEvent  rm.ProviderEvent
		expectedArgs   []interface{}
		deal           rm.ProviderDealState
		channelID      datatransfer.ChannelID
		dataAmount     uint64
		expectedResult datatransfer.VoucherResult
		expectedError  error
	}{
		"not tracked": {
			deal:      deal,
			channelID: shared_testutil.MakeTestChannelID(),
			noSend:    true,
		},
		"record block": {
			deal:          deal,
			channelID:     deal.ChannelID,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventBlockSent,
			expectedArgs:  []interface{}{deal.TotalSent + uint64(500)},
			dataAmount:    uint64(500),
		},
		"request payment": {
			deal:          deal,
			channelID:     deal.ChannelID,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentRequested,
			expectedArgs:  []interface{}{deal.TotalSent + defaultCurrentInterval},
			dataAmount:    defaultCurrentInterval,
			expectedError: datatransfer.ErrPause,
			expectedResult: &rm.DealResponse{
				ID:          deal.ID,
				Status:      rm.DealStatusFundsNeeded,
				PaymentOwed: defaultPaymentPerInterval,
			},
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			tn := testnodes.NewTestRetrievalProviderNode()
			fre := &fakeRevalidatorEnvironment{
				node:         tn,
				returnedDeal: data.deal,
				getError:     nil,
			}
			revalidator := requestvalidation.NewProviderRevalidator(fre)
			revalidator.TrackChannel(data.deal)
			voucherResult, err := revalidator.OnPullDataSent(data.channelID, data.dataAmount)
			require.Equal(t, data.expectedResult, voucherResult)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, data.expectedError.Error())
			}
			if !data.noSend {
				require.Len(t, fre.sentEvents, 1)
				event := fre.sentEvents[0]
				require.Equal(t, data.expectedID, event.ID)
				require.Equal(t, data.expectedEvent, event.Event)
				require.Equal(t, data.expectedArgs, event.Args)
			} else {
				require.Len(t, fre.sentEvents, 0)
			}
		})
	}
}

func TestOnComplete(t *testing.T) {
	deal := *makeDealState(rm.DealStatusOngoing)
	channelID := deal.ChannelID
	unpaidAmount := uint64(500)
	expectedEvents := []eventSent{
		{
			ID:    deal.Identifier(),
			Event: rm.ProviderEventBlockSent,
			Args:  []interface{}{deal.TotalSent + 500},
		},
		{
			ID:    deal.Identifier(),
			Event: rm.ProviderEventBlocksCompleted,
		},
		{
			ID:    deal.Identifier(),
			Event: rm.ProviderEventPaymentRequested,
			Args:  []interface{}{deal.TotalSent + 500},
		},
	}
	expectedError := datatransfer.ErrPause
	expectedResult := &rm.DealResponse{
		ID:          deal.ID,
		Status:      rm.DealStatusFundsNeededLastPayment,
		PaymentOwed: big.Mul(big.NewIntUnsigned(500), defaultPricePerByte),
	}
	tn := testnodes.NewTestRetrievalProviderNode()
	fre := &fakeRevalidatorEnvironment{
		node:         tn,
		returnedDeal: deal,
		getError:     nil,
	}
	revalidator := requestvalidation.NewProviderRevalidator(fre)
	revalidator.TrackChannel(deal)
	_, err := revalidator.OnPullDataSent(channelID, unpaidAmount)
	require.NoError(t, err)
	voucherResult, err := revalidator.OnComplete(channelID)
	require.Equal(t, expectedResult, voucherResult)
	require.EqualError(t, err, expectedError.Error())
	require.Equal(t, expectedEvents, fre.sentEvents)
}

func TestOnCompleteNoPaymentNeeded(t *testing.T) {
	deal := *makeDealState(rm.DealStatusOngoing)
	channelID := deal.ChannelID
	unpaidAmount := uint64(0)
	expectedEvents := []eventSent{
		{
			ID:    deal.Identifier(),
			Event: rm.ProviderEventBlockSent,
			Args:  []interface{}{deal.TotalSent},
		},
		{
			ID:    deal.Identifier(),
			Event: rm.ProviderEventBlocksCompleted,
		},
	}
	expectedResult := &rm.DealResponse{
		ID:     deal.ID,
		Status: rm.DealStatusCompleted,
	}
	tn := testnodes.NewTestRetrievalProviderNode()
	fre := &fakeRevalidatorEnvironment{
		node:         tn,
		returnedDeal: deal,
		getError:     nil,
	}
	revalidator := requestvalidation.NewProviderRevalidator(fre)
	revalidator.TrackChannel(deal)
	_, err := revalidator.OnPullDataSent(channelID, unpaidAmount)
	require.NoError(t, err)
	voucherResult, err := revalidator.OnComplete(channelID)
	require.Equal(t, expectedResult, voucherResult)
	require.NoError(t, err)
	require.Equal(t, expectedEvents, fre.sentEvents)
}

func TestRevalidate(t *testing.T) {
	payCh := address.TestAddress
	voucher := shared_testutil.MakeTestSignedVoucher()
	voucher.Amount = big.Add(defaultFundsReceived, defaultPaymentPerInterval)

	deal := *makeDealState(rm.DealStatusFundsNeeded)
	deal.TotalSent = defaultTotalSent + defaultCurrentInterval
	smallerPayment := abi.NewTokenAmount(400000)
	payment := &retrievalmarket.DealPayment{
		ID:             deal.ID,
		PaymentChannel: payCh,
		PaymentVoucher: voucher,
	}
	lastPaymentDeal := deal
	lastPaymentDeal.Status = rm.DealStatusFundsNeededLastPayment
	testCases := map[string]struct {
		configureTestNode func(tn *testnodes.TestRetrievalProviderNode)
		noSend            bool
		expectedID        rm.ProviderDealIdentifier
		expectedEvent     rm.ProviderEvent
		expectedArgs      []interface{}
		getError          error
		deal              rm.ProviderDealState
		channelID         datatransfer.ChannelID
		voucher           datatransfer.Voucher
		expectedResult    datatransfer.VoucherResult
		expectedError     error
	}{
		"not tracked": {
			deal:      deal,
			channelID: shared_testutil.MakeTestChannelID(),
			noSend:    true,
		},
		"not a payment voucher": {
			deal:          deal,
			channelID:     deal.ChannelID,
			noSend:        true,
			expectedError: errors.New("wrong voucher type"),
		},
		"error getting chain head": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				tn.ChainHeadError = errors.New("something went wrong")
			},
			deal:          deal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedError: errors.New("something went wrong"),
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventSaveVoucherFailed,
			expectedArgs:  []interface{}{errors.New("something went wrong")},
			expectedResult: &rm.DealResponse{
				ID:      deal.ID,
				Status:  rm.DealStatusErrored,
				Message: "something went wrong",
			},
		},
		"payment voucher error": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, abi.NewTokenAmount(0), errors.New("your money's no good here"))
			},
			deal:          deal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedError: errors.New("your money's no good here"),
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventSaveVoucherFailed,
			expectedArgs:  []interface{}{errors.New("your money's no good here")},
			expectedResult: &rm.DealResponse{
				ID:      deal.ID,
				Status:  rm.DealStatusErrored,
				Message: "your money's no good here",
			},
		},
		"not enough funds send": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, smallerPayment, nil)
			},
			deal:          deal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedError: datatransfer.ErrPause,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPartialPaymentReceived,
			expectedArgs:  []interface{}{smallerPayment},
			expectedResult: &rm.DealResponse{
				ID:          deal.ID,
				Status:      deal.Status,
				PaymentOwed: big.Sub(defaultPaymentPerInterval, smallerPayment),
			},
		},
		"it works": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
			},
			deal:          deal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
		},
		"it completes": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
			},
			deal:          lastPaymentDeal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
			expectedResult: &rm.DealResponse{
				ID:     deal.ID,
				Status: rm.DealStatusCompleted,
			},
		},
		"voucher already saved": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, big.Zero(), nil)
			},
			deal:          deal,
			channelID:     deal.ChannelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			tn := testnodes.NewTestRetrievalProviderNode()
			if data.configureTestNode != nil {
				data.configureTestNode(tn)
			}
			fre := &fakeRevalidatorEnvironment{
				node:         tn,
				returnedDeal: data.deal,
				getError:     data.getError,
			}
			revalidator := requestvalidation.NewProviderRevalidator(fre)
			revalidator.TrackChannel(data.deal)
			voucherResult, err := revalidator.Revalidate(data.channelID, data.voucher)
			require.Equal(t, data.expectedResult, voucherResult)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, data.expectedError.Error())
			}
			if !data.noSend {
				require.Len(t, fre.sentEvents, 1)
				event := fre.sentEvents[0]
				require.Equal(t, data.expectedID, event.ID)
				require.Equal(t, data.expectedEvent, event.Event)
				require.Equal(t, data.expectedArgs, event.Args)
			} else {
				require.Len(t, fre.sentEvents, 0)
			}
			tn.VerifyExpectations(t)
		})
	}
}

type eventSent struct {
	ID    rm.ProviderDealIdentifier
	Event rm.ProviderEvent
	Args  []interface{}
}
type fakeRevalidatorEnvironment struct {
	node           rm.RetrievalProviderNode
	sentEvents     []eventSent
	sendEventError error
	returnedDeal   rm.ProviderDealState
	getError       error
}

func (fre *fakeRevalidatorEnvironment) Node() rm.RetrievalProviderNode {
	return fre.node
}

func (fre *fakeRevalidatorEnvironment) SendEvent(dealID rm.ProviderDealIdentifier, evt rm.ProviderEvent, args ...interface{}) error {
	fre.sentEvents = append(fre.sentEvents, eventSent{dealID, evt, args})
	return fre.sendEventError
}

func (fre *fakeRevalidatorEnvironment) Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error) {
	return fre.returnedDeal, fre.getError
}

var dealID = retrievalmarket.DealID(10)
var defaultCurrentInterval = uint64(1000)
var defaultIntervalIncrease = uint64(500)
var defaultPricePerByte = abi.NewTokenAmount(500)
var defaultPaymentPerInterval = big.Mul(defaultPricePerByte, abi.NewTokenAmount(int64(defaultCurrentInterval)))
var defaultTotalSent = uint64(5000)
var defaultFundsReceived = abi.NewTokenAmount(2500000)

func makeDealState(status retrievalmarket.DealStatus) *retrievalmarket.ProviderDealState {
	channelID := shared_testutil.MakeTestChannelID()
	return &retrievalmarket.ProviderDealState{
		Status:          status,
		TotalSent:       defaultTotalSent,
		CurrentInterval: defaultCurrentInterval,
		FundsReceived:   defaultFundsReceived,
		ChannelID:       channelID,
		Receiver:        channelID.Initiator,
		DealProposal: retrievalmarket.DealProposal{
			ID:     dealID,
			Params: retrievalmarket.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
		},
	}
}
