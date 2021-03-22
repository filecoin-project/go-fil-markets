package providerstates

import (
	"context"
	"errors"
	"io"

	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

// ProviderDealEnvironment is a bridge to the environment a provider deal is executing in
// It provides access to relevant functionality on the retrieval provider
type ProviderDealEnvironment interface {
	// Node returns the node interface for this deal
	Node() rm.RetrievalProviderNode
	ReadIntoBlockstore(storeID multistore.StoreID, pieceData io.ReadCloser) error
	TrackTransfer(deal rm.ProviderDealState) error
	UntrackTransfer(deal rm.ProviderDealState) error
	DeleteStore(storeID multistore.StoreID) error
	ResumeDataTransfer(context.Context, datatransfer.ChannelID) error
	CloseDataTransfer(context.Context, datatransfer.ChannelID) error
}

func firstSuccessfulUnseal(ctx context.Context, node rm.RetrievalProviderNode, pieceInfo piecestore.PieceInfo) (io.ReadCloser, error) {
	lastErr := xerrors.New("no sectors found to unseal from")
	for _, deal := range pieceInfo.Deals {
		reader, err := node.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err == nil {
			return reader, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// UnsealData unseals the piece containing data for retrieval as needed
func UnsealData(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	reader, err := firstSuccessfulUnseal(ctx.Context(), environment.Node(), *deal.PieceInfo)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventUnsealError, err)
	}
	err = environment.ReadIntoBlockstore(deal.StoreID, reader)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventUnsealError, err)
	}
	return ctx.Trigger(rm.ProviderEventUnsealComplete)
}

// TrackTransfer resumes a deal so we can start sending data after its unsealed
func TrackTransfer(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.TrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	return nil
}

// UnpauseDeal resumes a deal so we can start sending data after its unsealed
func UnpauseDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.TrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	if deal.ChannelID != nil {
		err = environment.ResumeDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil {
			return ctx.Trigger(rm.ProviderEventDataTransferError, err)
		}
	}
	return nil
}

// CancelDeal clears a deal that went wrong for an unknown reason
func CancelDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	// Read next response (or fail)
	err := environment.UntrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	err = environment.DeleteStore(deal.StoreID)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventMultiStoreError, err)
	}
	if deal.ChannelID != nil {
		err = environment.CloseDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil && !errors.Is(err, statemachine.ErrTerminated) {
			return ctx.Trigger(rm.ProviderEventDataTransferError, err)
		}
	}
	return ctx.Trigger(rm.ProviderEventCancelComplete)
}

// CleanupDeal runs to do memory cleanup for an in progress deal
func CleanupDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.UntrackTransfer(deal)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventDataTransferError, err)
	}
	err = environment.DeleteStore(deal.StoreID)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventMultiStoreError, err)
	}
	return ctx.Trigger(rm.ProviderEventCleanupComplete)
}
