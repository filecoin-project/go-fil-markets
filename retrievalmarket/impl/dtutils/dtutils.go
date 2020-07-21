// Package dtutils provides event listeners for the client and provider to
// listen for events on the data transfer module and dispatch FSM events based on them
package dtutils

import (
	"errors"
	"math"

	logging "github.com/ipfs/go-log/v2"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("retrievalmarket_impl")

var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

// EventReceiver is any thing that can receive FSM events
type EventReceiver interface {
	Send(id interface{}, name fsm.EventName, args ...interface{}) (err error)
}

// ProviderDataTransferSubscriber is the function called when an event occurs in a data
// transfer received by a provider -- it reads the voucher to verify this event occurred
// in a storage market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func ProviderDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		dealProposal, ok := channelState.Voucher().(*rm.DealProposal)
		// if this event is for a transfer not related to storage, ignore
		if !ok {
			return
		}

		// data transfer events for progress do not affect deal state
		switch event.Code {
		case datatransfer.Accept:
			err := deals.Send(rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}, rm.ProviderEventDealAccepted, channelState.ChannelID())
			if err != nil {
				log.Errorf("processing dt event: %w", err)
			}
		case datatransfer.Error:
			err := deals.Send(rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}, rm.ProviderEventDataTransferError, errors.New(event.Message))
			if err != nil {
				log.Errorf("processing dt event: %w", err)
			}
		default:
		}

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}, rm.ProviderEventComplete)
			if err != nil {
				log.Errorf("processing dt event: %w", err)
			}
		}
	}
}

func clientEventForResponse(response *rm.DealResponse) (rm.ClientEvent, []interface{}) {
	switch response.Status {
	case rm.DealStatusRejected:
		return rm.ClientEventDealRejected, []interface{}{response.Message}
	case rm.DealStatusDealNotFound:
		return rm.ClientEventDealNotFound, []interface{}{response.Message}
	case rm.DealStatusAccepted:
		return rm.ClientEventDealAccepted, nil
	case rm.DealStatusFundsNeededUnseal:
		return rm.ClientEventPaymentRequested, []interface{}{response.PaymentOwed}
	case rm.DealStatusFundsNeededLastPayment:
		return rm.ClientEventLastPaymentRequested, []interface{}{response.PaymentOwed}
	case rm.DealStatusCompleted:
		return rm.ClientEventComplete, nil
	case rm.DealStatusFundsNeeded:
		return rm.ClientEventPaymentRequested, []interface{}{response.PaymentOwed}
	default:
		return rm.ClientEventUnknownResponseReceived, nil
	}
}

const noEvent = rm.ClientEvent(math.MaxUint64)

func clientEvent(event datatransfer.Event, channelState datatransfer.ChannelState) (rm.ClientEvent, []interface{}) {
	switch event.Code {
	case datatransfer.Progress:
		return rm.ClientEventBlocksReceived, []interface{}{channelState.Received()}
	case datatransfer.FinishTransfer:
		return rm.ClientEventAllBlocksReceived, nil
	case datatransfer.Cancel:
		return rm.ClientEventProviderCancelled, nil
	case datatransfer.NewVoucherResult:
		response, ok := channelState.LastVoucherResult().(*rm.DealResponse)
		if !ok {
			log.Errorf("unexpected voucher result received: %s", channelState.LastVoucher().Type())
			return noEvent, nil
		}
		return clientEventForResponse(response)
	case datatransfer.Error:
		return rm.ClientEventDataTransferError, []interface{}{errors.New(event.Message)}
	default:
	}

	return noEvent, nil
}

// ClientDataTransferSubscriber is the function called when an event occurs in a data
// transfer initiated on the client -- it reads the voucher to verify this even occurred
// in a storage market deal, then, based on the data transfer event that occurred, it dispatches
// an event to the appropriate state machine
func ClientDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		dealProposal, ok := channelState.Voucher().(*rm.DealProposal)

		// if this event is for a transfer not related to retrieval, ignore
		if !ok {
			return
		}

		retrievalEvent, params := clientEvent(event, channelState)
		if retrievalEvent == noEvent {
			return
		}

		// data transfer events for progress do not affect deal state
		err := deals.Send(dealProposal.ID, retrievalEvent, params...)
		if err != nil {
			log.Errorf("processing dt event: %w", err)
		}
	}
}
