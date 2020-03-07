package providerstates

import (
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"
)

// ProviderEvents are the events that can happen in a storage provider
var ProviderEvents = fsm.Events{
	fsm.Event(storagemarket.ProviderEventOpen).From(storagemarket.StorageDealUnknown).To(storagemarket.StorageDealValidating),
	fsm.Event(storagemarket.ProviderEventNodeErrored).FromAny().To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error calling node: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealRejected).From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("deal rejected: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealAccepted).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealProposalAccepted),
	fsm.Event(storagemarket.ProviderEventWaitingForManualData).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealWaitingForData),
	fsm.Event(storagemarket.ProviderEventDataTransferFailed).
		FromMany(storagemarket.StorageDealProposalAccepted, storagemarket.StorageDealTransferring).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error transferring data: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDataTransferInitiated).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealTransferring),
	fsm.Event(storagemarket.ProviderEventDataTransferCompleted).
		From(storagemarket.StorageDealTransferring).To(storagemarket.StorageDealVerifyData),
	fsm.Event(storagemarket.ProviderEventGeneratePieceCIDFailed).
		From(storagemarket.StorageDealVerifyData).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("generating piece committment: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventVerifiedData).
		FromMany(storagemarket.StorageDealVerifyData, storagemarket.StorageDealWaitingForData).To(storagemarket.StorageDealPublishing).
		Action(func(deal *storagemarket.MinerDeal, path filestore.Path) error {
			deal.PiecePath = path
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventSendResponseFailed).
		From(storagemarket.StorageDealPublishing).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("sending response to deal: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealPublished).
		From(storagemarket.StorageDealPublishing).To(storagemarket.StorageDealStaged).
		Action(func(deal *storagemarket.MinerDeal, dealID abi.DealID) error {
			deal.DealID = dealID
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventFileStoreErrored).FromAny().To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("accessing file store: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealHandoffFailed).From(storagemarket.StorageDealStaged).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("handing off deal to node: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealHandedOff).From(storagemarket.StorageDealStaged).To(storagemarket.StorageDealSealing),
	fsm.Event(storagemarket.ProviderEventDealActivationFailed).
		From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error activating deal: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealActivated).From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealActive),
	fsm.Event(storagemarket.ProviderEventPieceStoreErrored).From(storagemarket.StorageDealActive).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("accessing piece store: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealCompleted).From(storagemarket.StorageDealActive).To(storagemarket.StorageDealCompleted),
	fsm.Event(storagemarket.ProviderEventFailed).From(storagemarket.StorageDealFailing).To(storagemarket.StorageDealError),
}

// ProviderStateEntryFuncs are the handlers for different states in a storage client
var ProviderStateEntryFuncs = fsm.StateEntryFuncs{
	storagemarket.StorageDealValidating:       ValidateDealProposal,
	storagemarket.StorageDealProposalAccepted: TransferData,
	storagemarket.StorageDealVerifyData:       VerifyData,
	storagemarket.StorageDealPublishing:       PublishDeal,
	storagemarket.StorageDealStaged:           HandoffDeal,
	storagemarket.StorageDealSealing:          VerifyDealActivated,
	storagemarket.StorageDealActive:           RecordPieceInfo,
	storagemarket.StorageDealFailing:          FailDeal,
}
