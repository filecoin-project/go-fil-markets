package clientstates

import (
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// ClientEvents are the events that can happen in a storage client
var ClientEvents = fsm.Events{
	fsm.Event(storagemarket.ClientEventOpen).
		From(storagemarket.StorageDealUnknown).ToNoChange(),
	fsm.Event(storagemarket.ClientEventEnsureFundsFailed).
		From(storagemarket.StorageDealUnknown).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("adding market funds failed: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventFundsEnsured).
		From(storagemarket.StorageDealUnknown).To(storagemarket.StorageDealFundsEnsured),
	fsm.Event(storagemarket.ClientEventWriteProposalFailed).
		From(storagemarket.StorageDealFundsEnsured).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("sending proposal to storage provider failed: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealProposed).
		From(storagemarket.StorageDealFundsEnsured).To(storagemarket.StorageDealValidating),
	fsm.Event(storagemarket.ClientEventDealStreamLookupErrored).
		FromAny().To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("miner connection error: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventReadResponseFailed).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error reading Response message: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventResponseVerificationFailed).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.Message = "unable to verify signature on deal response"
			return nil
		}),
	fsm.Event(storagemarket.ClientEventResponseDealDidNotMatch).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, responseCid cid.Cid, proposalCid cid.Cid) error {
			deal.Message = xerrors.Errorf("miner responded to a wrong proposal: %s != %s", responseCid, proposalCid).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealRejected).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, state storagemarket.StorageDealStatus, reason string) error {
			deal.Message = xerrors.Errorf("deal failed: (State=%d) %s", state, reason).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealAccepted).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealProposalAccepted).
		Action(func(deal *storagemarket.ClientDeal, publishMessage *cid.Cid) error {
			deal.PublishMessage = publishMessage
			return nil
		}),
	fsm.Event(storagemarket.ClientEventStreamCloseError).
		FromAny().To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error attempting to close stream: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPublishFailed).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error validating deal published: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPublished).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealSealing).
		Action(func(deal *storagemarket.ClientDeal, dealID abi.DealID) error {
			deal.DealID = dealID
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealActivationFailed).
		From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error in deal activation: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealActivated).
		From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealActive),
	fsm.Event(storagemarket.ClientEventFailed).
		From(storagemarket.StorageDealFailing).To(storagemarket.StorageDealError),
}

// ClientStateEntryFuncs are the handlers for different states in a storage client
var ClientStateEntryFuncs = fsm.StateEntryFuncs{
	storagemarket.StorageDealUnknown:          EnsureFunds,
	storagemarket.StorageDealFundsEnsured:     ProposeDeal,
	storagemarket.StorageDealValidating:       VerifyDealResponse,
	storagemarket.StorageDealProposalAccepted: ValidateDealPublished,
	storagemarket.StorageDealSealing:          VerifyDealActivated,
	storagemarket.StorageDealFailing:          FailDeal,
}
