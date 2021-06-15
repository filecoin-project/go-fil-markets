package providerstates

import (
	"context"
	"errors"
	"io"

	dagstore "github.com/filecoin-project/dagstore/store"
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
	// prefer an unsealed sector containing the piece if one exists
	for _, deal := range pieceInfo.Deals {
		isUnsealed, err := node.IsUnsealed(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err != nil {
			continue
		}
		if isUnsealed {
			// UnsealSector will NOT unseal a sector if we already have an unsealed copy lying around.
			reader, err := node.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
			if err == nil {
				return reader, nil
			}
		}
	}

	lastErr := xerrors.New("no sectors found to unseal from")
	// if there is no unsealed sector containing the piece, just read the piece from the first sector we are able to unseal.
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
func UnsealData(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState, ds dagstore.DAGStore) error {
	pieceCID := deal.PieceInfo.PieceCID.String()

	// Can the sharded DAG Store serve a retrieval for this Piece ?
	// This will be false ONLY if the deal is no longer active because even if the unsealed
	// file for a deal has been deleted, the dag store can serve the retrieval by unsealing the file again
	// since it's been told how to do so by calling the unsealing function when the shard is first activated.
	b, err := ds.IsShardActive(pieceCID)
	if err != nil {
		return nil
	}

	var rb dagstore.ReadOnlyBlockStore
	if !b {
		// This should ideally NEVER happen because the shard will be activated when the storage deal is published
		// and will be removed ONLY when the storage deal expires.

		// However, if it does happen, we need to activate the shard for this piece in the DAG Store.
		// This call will block till the shard is activated i.e. an unsealed copy is fetched and indexed.
		// It is possible that the DAG Store already has the index for this piece from a previous activation
		// in which case we wont have to index it again.
		// The one problem with this API and it's usage with the `GetShardReadOnlyBlockstore` call below is that the
		// DAG Store will never memoize the unsealed file and so each call to `GetShardReadOnlyBlockstore` will result into one more fetch
		// of the unsealed file but that should probably be okay as it seems to be a (Save space) vs (save time) tradeoff.
		if err := ds.ActivateShard(pieceCID, &unsealedFetcher{
			environment.Node(),
			*deal.PieceInfo,
		}); err != nil {
			return err
		}
	}

	// get a read only blockstore that can be used to serve random access for the piece data here.
	rb, err = ds.GetShardReadOnlyBlockstore(pieceCID)
	if err != nil || rb == nil {
		return err
	}

	// TODO Confugure graphsync to use the read ONLY block store created above and then CLOSE the blockstore when done.
	err = environment.ReadIntoBlockstore(deal.StoreID, reader)
	if err != nil {
		return ctx.Trigger(rm.ProviderEventUnsealError, err)
	}
	return ctx.Trigger(rm.ProviderEventUnsealComplete)
}

type unsealedFetcher struct {
	rm        rm.RetrievalProviderNode
	pieceInfo piecestore.PieceInfo
}

func (u *unsealedFetcher) Fetch(ctx context.Context) (io.ReadCloser, error) {
	return firstSuccessfulUnseal(ctx, u.rm, u.pieceInfo)
}

func (u *unsealedFetcher) FindIfActive() bool {
	// TODO Is the deal still active ?
	return true
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
