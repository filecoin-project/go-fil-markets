package dtutils_test

import (
	"errors"
	"math/rand"
	"testing"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/stretchr/testify/require"
)

func TestProviderDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	testPeers := shared_testutil.GeneratePeers(2)
	transferID := datatransfer.TransferID(rand.Uint64())
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		state         shared_testutil.TestChannelParams
		ignored       bool
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a retrieval voucher": {
			ignored: true,
		},
		"accept": {
			code: datatransfer.Accept,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDealAccepted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{ID: transferID, Initiator: testPeers[1], Responder: testPeers[0]}},
		},
		"error": {
			code:    datatransfer.Error,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("something went wrong")},
		},
		"completed": {
			code: datatransfer.ResumeResponder,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Completed},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventComplete,
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ProviderDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(data.state))
			if !data.ignored {
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, data.expectedID)
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				require.False(t, fdg.called)
			}
		})
	}

}
func TestClientDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	paymentOwed := shared_testutil.MakeTestTokenAmount()
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		state         shared_testutil.TestChannelParams
		ignored       bool
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a retrieval voucher": {
			ignored: true,
		},
		"progress": {
			code: datatransfer.Progress,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing,
				Received: 1000},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventBlocksReceived,
			expectedArgs:  []interface{}{uint64(1000)},
		},
		"finish transfer": {
			code: datatransfer.FinishTransfer,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.TransferFinished},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventAllBlocksReceived,
		},
		"cancel": {
			code: datatransfer.Cancel,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventProviderCancelled,
		},
		"new voucher result - rejected": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusRejected,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealRejected,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - not found": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusDealNotFound,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealNotFound,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - accepted": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusAccepted,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealAccepted,
		},
		"new voucher result - funds needed last payment": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeededLastPayment,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventLastPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - completed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusCompleted,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.ResponderCompleted},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventComplete,
		},
		"new voucher result - funds needed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeeded,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - unexpected response": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusPaymentChannelAddingFunds,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventUnknownResponseReceived,
		},
		"error": {
			code:    datatransfer.Error,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("something went wrong")},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ClientDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(data.state))
			if !data.ignored {
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, data.expectedID)
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				require.False(t, fdg.called)
			}
		})
	}
}

type fakeDealGroup struct {
	returnedErr error
	called      bool
	lastID      interface{}
	lastEvent   fsm.EventName
	lastArgs    []interface{}
}

func (fdg *fakeDealGroup) Send(id interface{}, name fsm.EventName, args ...interface{}) (err error) {
	fdg.lastID = id
	fdg.lastEvent = name
	fdg.lastArgs = args
	fdg.called = true
	return fdg.returnedErr
}
