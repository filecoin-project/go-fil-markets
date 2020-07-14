package providerstates

import (
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

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
		From(rm.DealStatusNew).To(rm.DealStatusUnsealing).
		Action(func(deal *rm.ProviderDealState, channelID datatransfer.ChannelID) error {
			deal.ChannelID = channelID
			return nil
		}),

	//unsealing
	fsm.Event(rm.ProviderEventUnsealError).
		From(rm.DealStatusUnsealing).To(rm.DealStatusFailing).
		Action(recordError),
	fsm.Event(rm.ProviderEventUnsealComplete).
		From(rm.DealStatusUnsealing).To(rm.DealStatusUnsealed),

	// start sending data
	fsm.Event(rm.ProviderEventUnpauseDeal).
		From(rm.DealStatusUnsealed).To(rm.DealStatusOngoing),

	// receiving blocks
	fsm.Event(rm.ProviderEventBlockSent).
		FromMany(rm.DealStatusOngoing).ToNoChange().
		Action(func(deal *rm.ProviderDealState, totalSent uint64) error {
			deal.TotalSent = totalSent
			return nil
		}),
	fsm.Event(rm.ProviderEventBlocksCompleted).
		FromMany(rm.DealStatusOngoing).To(rm.DealStatusBlocksComplete),

	// request payment
	fsm.Event(rm.ProviderEventPaymentRequested).
		FromMany(rm.DealStatusOngoing).To(rm.DealStatusFundsNeeded).
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusFundsNeededLastPayment).
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
		Action(func(deal *rm.ProviderDealState, fundsReceived abi.TokenAmount) error {
			deal.FundsReceived = big.Add(deal.FundsReceived, fundsReceived)
			deal.CurrentInterval += deal.PaymentIntervalIncrease
			return nil
		}),

	// completing
	fsm.Event(rm.ProviderEventComplete).From(rm.DealStatusFinalizing).To(rm.DealStatusCompleted),

	// data transfer errors
	fsm.Event(rm.ProviderEventDataTransferError).
		FromAny().To(rm.DealStatusErrored).
		Action(recordError),
}

// ProviderStateEntryFuncs are the handlers for different states in a retrieval provider
var ProviderStateEntryFuncs = fsm.StateEntryFuncs{
	rm.DealStatusUnsealing: UnsealData,
	rm.DealStatusUnsealed:  UnpauseDeal,
	rm.DealStatusFailing:   CancelDeal,
}
