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
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestOnPushDataReceived(t *testing.T) {
	fre := &fakeRevalidatorEnvironment{}
	revalidator := requestvalidation.NewProviderRevalidator(fre)
	channelID := shared_testutil.MakeTestChannelID()
	handled, voucherResult, err := revalidator.OnPushDataReceived(channelID, rand.Uint64())
	require.False(t, handled)
	require.NoError(t, err)
	require.Nil(t, voucherResult)
}

func TestOnPullDataSent(t *testing.T) {
	deal := *makeDealState(rm.DealStatusOngoing)
	dealZeroPricePerByte := deal
	dealZeroPricePerByte.PricePerByte = big.Zero()
	legacyDeal := deal
	legacyDeal.LegacyProtocol = true
	testCases := map[string]struct {
		noSend          bool
		expectedID      rm.ProviderDealIdentifier
		expectedEvent   rm.ProviderEvent
		expectedArgs    []interface{}
		deal            rm.ProviderDealState
		channelID       datatransfer.ChannelID
		dataAmount      uint64
		expectedHandled bool
		expectedResult  datatransfer.VoucherResult
		expectedError   error
	}{
		"not tracked": {
			deal:      deal,
			channelID: shared_testutil.MakeTestChannelID(),
			noSend:    true,
		},
		"record block": {
			deal:            deal,
			channelID:       *deal.ChannelID,
			expectedID:      deal.Identifier(),
			expectedEvent:   rm.ProviderEventBlockSent,
			expectedArgs:    []interface{}{deal.TotalSent + uint64(500)},
			expectedHandled: true,
			dataAmount:      uint64(500),
		},
		"record block zero price per byte": {
			deal:            dealZeroPricePerByte,
			channelID:       *dealZeroPricePerByte.ChannelID,
			expectedID:      dealZeroPricePerByte.Identifier(),
			expectedEvent:   rm.ProviderEventBlockSent,
			expectedArgs:    []interface{}{dealZeroPricePerByte.TotalSent + uint64(500)},
			expectedHandled: true,
			dataAmount:      uint64(500),
		},
		"request payment": {
			deal:          deal,
			channelID:     *deal.ChannelID,
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
			expectedHandled: true,
		},
		"request payment, legacy": {
			deal:          legacyDeal,
			channelID:     *legacyDeal.ChannelID,
			expectedID:    legacyDeal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentRequested,
			expectedArgs:  []interface{}{legacyDeal.TotalSent + defaultCurrentInterval},
			dataAmount:    defaultCurrentInterval,
			expectedError: datatransfer.ErrPause,
			expectedResult: &migrations.DealResponse0{
				ID:          legacyDeal.ID,
				Status:      rm.DealStatusFundsNeeded,
				PaymentOwed: defaultPaymentPerInterval,
			},
			expectedHandled: true,
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
			handled, voucherResult, err := revalidator.OnPullDataSent(data.channelID, data.dataAmount)
			require.Equal(t, data.expectedHandled, handled)
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
	dealZeroPricePerByte := deal
	dealZeroPricePerByte.PricePerByte = big.Zero()
	legacyDeal := deal
	legacyDeal.LegacyProtocol = true
	channelID := *deal.ChannelID
	testCases := map[string]struct {
		expectedEvents []eventSent
		deal           rm.ProviderDealState
		channelID      datatransfer.ChannelID
		expectedResult datatransfer.VoucherResult
		expectedError  error
		unpaidAmount   uint64
	}{
		"unpaid money": {
			unpaidAmount: uint64(500),
			expectedEvents: []eventSent{
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
			},
			expectedError: datatransfer.ErrPause,
			expectedResult: &rm.DealResponse{
				ID:          deal.ID,
				Status:      rm.DealStatusFundsNeededLastPayment,
				PaymentOwed: big.Mul(big.NewIntUnsigned(500), defaultPricePerByte),
			},
			deal:      deal,
			channelID: channelID,
		},
		"unpaid money, legacyDeal": {
			unpaidAmount: uint64(500),
			expectedEvents: []eventSent{
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
			},
			expectedError: datatransfer.ErrPause,
			expectedResult: &migrations.DealResponse0{
				ID:          deal.ID,
				Status:      rm.DealStatusFundsNeededLastPayment,
				PaymentOwed: big.Mul(big.NewIntUnsigned(500), defaultPricePerByte),
			},
			deal:      legacyDeal,
			channelID: channelID,
		},
		"all funds paid": {
			unpaidAmount: uint64(0),
			expectedEvents: []eventSent{
				{
					ID:    deal.Identifier(),
					Event: rm.ProviderEventBlockSent,
					Args:  []interface{}{deal.TotalSent},
				},
				{
					ID:    deal.Identifier(),
					Event: rm.ProviderEventBlocksCompleted,
				},
			},
			expectedResult: &rm.DealResponse{
				ID:     deal.ID,
				Status: rm.DealStatusCompleted,
			},
			deal:      deal,
			channelID: channelID,
		},
		"all funds paid zero price per byte": {
			unpaidAmount: uint64(0),
			expectedEvents: []eventSent{
				{
					ID:    dealZeroPricePerByte.Identifier(),
					Event: rm.ProviderEventBlockSent,
					Args:  []interface{}{dealZeroPricePerByte.TotalSent},
				},
				{
					ID:    dealZeroPricePerByte.Identifier(),
					Event: rm.ProviderEventBlocksCompleted,
				},
			},
			expectedResult: &rm.DealResponse{
				ID:     dealZeroPricePerByte.ID,
				Status: rm.DealStatusCompleted,
			},
			deal:      dealZeroPricePerByte,
			channelID: channelID,
		},
		"all funds paid, legacyDeal": {
			unpaidAmount: uint64(0),
			expectedEvents: []eventSent{
				{
					ID:    deal.Identifier(),
					Event: rm.ProviderEventBlockSent,
					Args:  []interface{}{deal.TotalSent},
				},
				{
					ID:    deal.Identifier(),
					Event: rm.ProviderEventBlocksCompleted,
				},
			},
			expectedResult: &migrations.DealResponse0{
				ID:     deal.ID,
				Status: rm.DealStatusCompleted,
			},
			deal:      legacyDeal,
			channelID: channelID,
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
			_, _, err := revalidator.OnPullDataSent(data.channelID, data.unpaidAmount)
			require.NoError(t, err)
			handled, voucherResult, err := revalidator.OnComplete(data.channelID)
			require.True(t, handled)
			require.Equal(t, data.expectedResult, voucherResult)
			if data.expectedError != nil {
				require.EqualError(t, err, data.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, data.expectedEvents, fre.sentEvents)
		})
	}
}

func TestRevalidate(t *testing.T) {
	payCh := address.TestAddress
	voucher := shared_testutil.MakeTestSignedVoucher()
	voucher.Amount = big.Add(defaultFundsReceived, defaultPaymentPerInterval)

	deal := *makeDealState(rm.DealStatusFundsNeeded)
	deal.TotalSent = defaultTotalSent + defaultCurrentInterval
	channelID := *deal.ChannelID
	smallerPayment := abi.NewTokenAmount(400000)
	payment := &retrievalmarket.DealPayment{
		ID:             deal.ID,
		PaymentChannel: payCh,
		PaymentVoucher: voucher,
	}
	legacyPayment := &migrations.DealPayment0{
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
			channelID:     channelID,
			noSend:        true,
			expectedError: errors.New("wrong voucher type"),
		},
		"error getting chain head": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				tn.ChainHeadError = errors.New("something went wrong")
			},
			deal:          deal,
			channelID:     channelID,
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
		"error getting chain head, legacyPayment": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				tn.ChainHeadError = errors.New("something went wrong")
			},
			deal:          deal,
			channelID:     channelID,
			voucher:       legacyPayment,
			expectedError: errors.New("something went wrong"),
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventSaveVoucherFailed,
			expectedArgs:  []interface{}{errors.New("something went wrong")},
			expectedResult: &migrations.DealResponse0{
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
			channelID:     channelID,
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
		"payment voucher error, legacy payment": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, abi.NewTokenAmount(0), errors.New("your money's no good here"))
			},
			deal:          deal,
			channelID:     channelID,
			voucher:       legacyPayment,
			expectedError: errors.New("your money's no good here"),
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventSaveVoucherFailed,
			expectedArgs:  []interface{}{errors.New("your money's no good here")},
			expectedResult: &migrations.DealResponse0{
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
			channelID:     channelID,
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
		"not enough funds send, legacyPayment": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, smallerPayment, nil)
			},
			deal:          deal,
			channelID:     channelID,
			voucher:       legacyPayment,
			expectedError: datatransfer.ErrPause,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPartialPaymentReceived,
			expectedArgs:  []interface{}{smallerPayment},
			expectedResult: &migrations.DealResponse0{
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
			channelID:     channelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
			expectedError: datatransfer.ErrResume,
		},

		"it completes": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
			},
			deal:          lastPaymentDeal,
			channelID:     channelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
			expectedError: datatransfer.ErrResume,
			expectedResult: &rm.DealResponse{
				ID:     deal.ID,
				Status: rm.DealStatusCompleted,
			},
		},
		"it completes, legacy payment": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, defaultPaymentPerInterval, nil)
			},
			deal:          lastPaymentDeal,
			channelID:     channelID,
			voucher:       legacyPayment,
			expectedID:    deal.Identifier(),
			expectedError: datatransfer.ErrResume,
			expectedEvent: rm.ProviderEventPaymentReceived,
			expectedArgs:  []interface{}{defaultPaymentPerInterval},
			expectedResult: &migrations.DealResponse0{
				ID:     deal.ID,
				Status: rm.DealStatusCompleted,
			},
		},
		"voucher already saved": {
			configureTestNode: func(tn *testnodes.TestRetrievalProviderNode) {
				_ = tn.ExpectVoucher(payCh, voucher, nil, defaultPaymentPerInterval, big.Zero(), nil)
			},
			deal:          deal,
			channelID:     channelID,
			voucher:       payment,
			expectedID:    deal.Identifier(),
			expectedError: datatransfer.ErrResume,
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
		ChannelID:       &channelID,
		Receiver:        channelID.Initiator,
		DealProposal: retrievalmarket.DealProposal{
			ID:     dealID,
			Params: retrievalmarket.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
		},
	}
}
