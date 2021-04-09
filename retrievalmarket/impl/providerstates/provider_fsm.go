package providerstates

import (
	"fmt"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

func recordError(deal *rm.ProviderDealState, err error) error {
	deal.Message = err.Error()
	return nil
}

// ProviderEvents are the events that can happen in a retrieval provider
var ProviderEvents = fsm.Events{
	// receiving new deal
	fsm.Event(rm.ProviderEventOpen).
		From(rm.DealStatusNew).ToNoChange().
		Action(
			func(deal *rm.ProviderDealState) error {
				deal.TotalSent = 0
				deal.FundsReceived = abi.NewTokenAmount(0)
				deal.CurrentInterval = deal.PaymentInterval
				return nil
			},
		),

	// accepting
	fsm.Event(rm.ProviderEventDealAccepted).
		From(rm.DealStatusFundsNeededUnseal).ToNoChange().
		From(rm.DealStatusNew).To(rm.DealStatusUnsealing).
		Action(func(deal *rm.ProviderDealState, channelID datatransfer.ChannelID) error {
			deal.ChannelID = &channelID
			return nil
		}),

	//unsealing
	fsm.Event(rm.ProviderEventUnsealError).
		From(rm.DealStatusUnsealing).To(rm.DealStatusFailing).
		Action(recordError),
	fsm.Event(rm.ProviderEventUnsealComplete).
		From(rm.DealStatusUnsealing).To(rm.DealStatusUnsealed),

	// restart
	fsm.Event(rm.ProviderEventRestart).
		FromMany(rm.DealStatusOngoing, rm.DealStatusFundsNeeded).To(rm.DealStatusProviderRestarted).
		// TODO What's the right thing to do here ?
		From(rm.DealStatusProviderRestarted).ToNoChange().
		// TODO Support other states later here
		FromAny().To(rm.DealStatusErrored).
		Action(func(deal *rm.ProviderDealState) error {
			// set TotalSent to zero and update as we queue blocks we've already sent
			deal.TotalSent = 0
			// how do we update the current interval here ?

			return nil
		}),

	// duplicate block traversed during restart
	fsm.Event(rm.ProviderEventDuplicateTraversed).
		From(rm.DealStatusProviderRestarted).ToNoChange().
		// we should NOT see a duplicate if we're not in the restart state.
		FromAny().To(rm.DealStatusErrored).
		Action(func(deal *rm.ProviderDealState, totalSent uint64) error {
			deal.TotalSent = totalSent
			return nil
		}),

	// receiving blocks
	fsm.Event(rm.ProviderEventBlockSent).
		From(rm.DealStatusProviderRestarted).To(rm.DealStatusOngoing).
		FromMany(rm.DealStatusOngoing).ToNoChange().
		From(rm.DealStatusUnsealed).To(rm.DealStatusOngoing).
		Action(func(deal *rm.ProviderDealState, totalSent uint64) error {
			if deal.Status == rm.DealStatusProviderRestarted {
				fmt.Printf("\n provider caught up at totalSent=%d", deal.TotalSent)
			}

			deal.TotalSent = totalSent
			return nil
		}),
	fsm.Event(rm.ProviderEventBlocksCompleted).
		FromMany(rm.DealStatusOngoing).To(rm.DealStatusBlocksComplete),

	// request payment
	fsm.Event(rm.ProviderEventPaymentRequested).
		From(rm.DealStatusProviderRestarted).To(rm.DealStatusFundsNeeded).
		FromMany(rm.DealStatusOngoing, rm.DealStatusUnsealed).To(rm.DealStatusFundsNeeded).
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusFundsNeededLastPayment).
		From(rm.DealStatusNew).To(rm.DealStatusFundsNeededUnseal).
		Action(func(deal *rm.ProviderDealState, totalSent uint64) error {
			deal.TotalSent = totalSent
			return nil
		}),

	// receive and process payment
	fsm.Event(rm.ProviderEventSaveVoucherFailed).
		FromMany(rm.DealStatusFundsNeeded, rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusFailing).
		Action(recordError),
	fsm.Event(rm.ProviderEventPartialPaymentReceived).
		FromMany(rm.DealStatusFundsNeeded, rm.DealStatusFundsNeededLastPayment).ToNoChange().
		Action(func(deal *rm.ProviderDealState, fundsReceived abi.TokenAmount) error {
			deal.FundsReceived = big.Add(deal.FundsReceived, fundsReceived)
			return nil
		}),
	fsm.Event(rm.ProviderEventPaymentReceived).
		From(rm.DealStatusFundsNeeded).To(rm.DealStatusOngoing).
		From(rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusFinalizing).
		From(rm.DealStatusFundsNeededUnseal).To(rm.DealStatusUnsealing).
		Action(func(deal *rm.ProviderDealState, fundsReceived abi.TokenAmount) error {
			deal.FundsReceived = big.Add(deal.FundsReceived, fundsReceived)

			// only update interval if the payment is for bytes and not for unsealing.
			if deal.Status != rm.DealStatusFundsNeededUnseal {
				deal.CurrentInterval += deal.PaymentIntervalIncrease
			}
			return nil
		}),

	// completing
	fsm.Event(rm.ProviderEventComplete).FromMany(rm.DealStatusBlocksComplete, rm.DealStatusFinalizing).To(rm.DealStatusCompleting),
	fsm.Event(rm.ProviderEventCleanupComplete).From(rm.DealStatusCompleting).To(rm.DealStatusCompleted),

	// Cancellation / Error cleanup
	fsm.Event(rm.ProviderEventCancelComplete).
		From(rm.DealStatusCancelling).To(rm.DealStatusCancelled).
		From(rm.DealStatusFailing).To(rm.DealStatusErrored),

	// data transfer errors
	fsm.Event(rm.ProviderEventDataTransferError).
		FromAny().To(rm.DealStatusErrored).
		Action(recordError),

	// multistore errors
	fsm.Event(rm.ProviderEventMultiStoreError).
		FromAny().To(rm.DealStatusErrored).
		Action(recordError),

	fsm.Event(rm.ProviderEventClientCancelled).
		From(rm.DealStatusFailing).ToJustRecord().
		From(rm.DealStatusCancelling).ToJustRecord().
		FromAny().To(rm.DealStatusCancelling).Action(
		func(deal *rm.ProviderDealState) error {
			if deal.Status != rm.DealStatusFailing {
				deal.Message = "Client cancelled retrieval"
			}
			return nil
		},
	),
}

// ProviderStateEntryFuncs are the handlers for different states in a retrieval provider
var ProviderStateEntryFuncs = fsm.StateEntryFuncs{
	rm.DealStatusFundsNeededUnseal: TrackTransfer,
	rm.DealStatusUnsealing:         UnsealData,
	rm.DealStatusUnsealed:          UnpauseDeal,
	rm.DealStatusFailing:           CancelDeal,
	rm.DealStatusCancelling:        CancelDeal,
	rm.DealStatusCompleting:        CleanupDeal,
}

// ProviderFinalityStates are the terminal states for a retrieval provider
var ProviderFinalityStates = []fsm.StateKey{
	rm.DealStatusErrored,
	rm.DealStatusCompleted,
	rm.DealStatusCancelled,
}
