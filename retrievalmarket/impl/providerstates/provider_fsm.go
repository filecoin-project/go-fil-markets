package providerstates

import (
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"golang.org/x/xerrors"
)

func recordError(deal *rm.ProviderDealState, err error) error {
	deal.Message = err.Error()
	return nil
}

// ProviderEvents are the events that can happen in a retrieval provider
var ProviderEvents = fsm.Events{
	fsm.Event(rm.ProviderEventOpen).
		From(rm.DealStatusNew).ToNoChange().
		WithCallback(
			func(deal *rm.ProviderDealState) error {
				deal.TotalSent = 0
				deal.FundsReceived = abi.NewTokenAmount(0)
				return nil
			},
		),
	fsm.Event(rm.ProviderEventWriteResponseFailed).
		FromAny().To(rm.DealStatusErrored).
		WithCallback(func(deal *rm.ProviderDealState, err error) error {
			deal.Message = xerrors.Errorf("writing deal response: %w", err).Error()
			return nil
		}),
	fsm.Event(rm.ProviderEventReadPaymentFailed).
		FromAny().To(rm.DealStatusErrored).
		WithCallback(recordError),
	fsm.Event(rm.ProviderEventGetPieceSizeErrored).
		From(rm.DealStatusNew).To(rm.DealStatusFailed).
		WithCallback(recordError),
	fsm.Event(rm.ProviderEventDealNotFound).
		From(rm.DealStatusNew).To(rm.DealStatusDealNotFound).
		WithCallback(func(deal *rm.ProviderDealState) error {
			deal.Message = rm.ErrNotFound.Error()
			return nil
		}),
	fsm.Event(rm.ProviderEventDealRejected).
		From(rm.DealStatusNew).To(rm.DealStatusRejected).
		WithCallback(recordError),
	fsm.Event(rm.ProviderEventDealAccepted).
		From(rm.DealStatusNew).To(rm.DealStatusAccepted).
		WithCallback(func(deal *rm.ProviderDealState, dealProposal rm.DealProposal) error {
			deal.DealProposal = dealProposal
			deal.CurrentInterval = deal.PaymentInterval
			return nil
		}),
	fsm.Event(rm.ProviderEventBlockErrored).
		FromMany(rm.DealStatusAccepted, rm.DealStatusOngoing).To(rm.DealStatusFailed).
		WithCallback(recordError),
	fsm.Event(rm.ProviderEventBlocksCompleted).
		FromMany(rm.DealStatusAccepted, rm.DealStatusOngoing).To(rm.DealStatusBlocksComplete),
	fsm.Event(rm.ProviderEventPaymentRequested).
		FromMany(rm.DealStatusAccepted, rm.DealStatusOngoing).To(rm.DealStatusFundsNeeded).
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusFundsNeededLastPayment).
		WithCallback(func(deal *rm.ProviderDealState, totalSent uint64) error {
			deal.TotalSent = totalSent
			return nil
		}),
	fsm.Event(rm.ProviderEventSaveVoucherFailed).
		FromMany(rm.DealStatusFundsNeeded, rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusFailed).
		WithCallback(recordError),
	fsm.Event(rm.ProviderEventPartialPaymentReceived).
		FromMany(rm.DealStatusFundsNeeded, rm.DealStatusFundsNeededLastPayment).ToNoChange().
		WithCallback(func(deal *rm.ProviderDealState, fundsReceived abi.TokenAmount) error {
			deal.FundsReceived = big.Add(deal.FundsReceived, fundsReceived)
			return nil
		}),
	fsm.Event(rm.ProviderEventPaymentReceived).
		From(rm.DealStatusFundsNeeded).To(rm.DealStatusOngoing).
		From(rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusFinalizing).
		WithCallback(func(deal *rm.ProviderDealState, fundsReceived abi.TokenAmount) error {
			deal.FundsReceived = big.Add(deal.FundsReceived, fundsReceived)
			deal.CurrentInterval += deal.PaymentIntervalIncrease
			return nil
		}),
	fsm.Event(rm.ProviderEventComplete).
		From(rm.DealStatusFinalizing).To(rm.DealStatusCompleted),
}

// ProviderHandlers are the handlers for different states in a retrieval provider
var ProviderHandlers = fsm.StateHandlers{
	rm.DealStatusNew:                    ReceiveDeal,
	rm.DealStatusFailed:                 SendFailResponse,
	rm.DealStatusRejected:               SendFailResponse,
	rm.DealStatusDealNotFound:           SendFailResponse,
	rm.DealStatusOngoing:                SendBlocks,
	rm.DealStatusAccepted:               SendBlocks,
	rm.DealStatusFundsNeeded:            ProcessPayment,
	rm.DealStatusFundsNeededLastPayment: ProcessPayment,
	rm.DealStatusFinalizing:             Finalize,
}
