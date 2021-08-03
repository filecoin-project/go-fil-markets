package storageimpl

import (
	"context"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/stores"
)

var shardRegMarker = ".shard-registration-complete"

// ShardMigrator is used to register all deals that are in the sealing / sealed
// state with the DAG store as shards.
// It will only run once on startup, from that point forward deals will be
// registered as shards as part of the deals FSM.
type ShardMigrator struct {
	providerAddr   address.Address
	markerFilePath string
	dagStore       stores.DAGStoreWrapper

	pieceStore piecestore.PieceStore
	spn        storagemarket.StorageProviderNode
}

func NewShardMigrator(
	maddr address.Address,
	dagStorePath string,
	dagStore stores.DAGStoreWrapper,
	pieceStore piecestore.PieceStore,
	spn storagemarket.StorageProviderNode,
) *ShardMigrator {
	return &ShardMigrator{
		providerAddr:   maddr,
		markerFilePath: filepath.Join(dagStorePath, shardRegMarker),
		dagStore:       dagStore,
		pieceStore:     pieceStore,
		spn:            spn,
	}
}

func (r *ShardMigrator) registerShards(ctx context.Context, deals []storagemarket.MinerDeal) error {
	log := log.Named("migrator")

	// Check if all deals have already been registered as shards
	isComplete, err := r.registrationComplete()
	if err != nil {
		return xerrors.Errorf("failed to get shard registration status: %w", err)
	}
	if isComplete {
		log.Info("no shard migration necessary; already marked complete")
		// All deals have been registered as shards, bail out
		return nil
	}

	log.Infow("registering shards for all active deals in sealing subsystem", "count", len(deals))

	inSealingSubsystem := make(map[fsm.StateKey]struct{}, len(providerstates.StatesKnownBySealingSubsystem))
	for _, s := range providerstates.StatesKnownBySealingSubsystem {
		inSealingSubsystem[s] = struct{}{}
	}

	// channel where results will be received, and channel where the total
	// number of registered shards will be sent.
	resch := make(chan dagstore.ShardResult, 32)
	totalCh := make(chan int)

	// Start making progress consuming results. We won't know how many to
	// actually consume until we register all shards.
	//
	// If there are any problems registering shards, just log an error
	go func() {
		var total = math.MaxInt64
		var res dagstore.ShardResult
		for rcvd := 0; rcvd < total; {
			select {
			case total = <-totalCh:
				// we now know the total number of registered shards
				// nullify so that we no longer consume from it after closed.
				close(totalCh)
				totalCh = nil
			case res = <-resch:
				rcvd++
				if res.Error == nil {
					log.Infow("async shard registration completed successfully", "shard_key", res.Key)
				} else {
					log.Warnw("async shard registration failed", "shard_key", res.Key, "error", res.Error)
				}
			}
		}
		log.Infow("all migrated shards initialized")
	}()

	// Filter for deals that are handed off.
	// If the deal has not yet been handed off to the sealing subsystem, we
	// don't need to call RegisterShard in this migration; RegisterShard will
	// be called in the new code once the deal reaches the state where it's
	// handed off to the sealing subsystem.
	var registered int
	for _, deal := range deals {
		if deal.Ref.PieceCid == nil {
			log.Warnw("deal has nil piece CID; skipping", "deal_id", deal.DealID)
			continue
		}
		// Filter for deals that have been handed off to the sealing subsystem
		if _, ok := inSealingSubsystem[deal.State]; !ok {
			log.Infow("deal not ready; skipping", "deal_id", deal.DealID)
			continue
		}

		// check if we have an unsealed sector for this piece.
		pcid := *deal.Ref.PieceCid
		pinfo, err := r.pieceStore.GetPieceInfo(pcid)
		if err != nil {
			return xerrors.Errorf("failed to get piece info for a deal piece %s: %w", pcid, err)
		}
		if len(pinfo.Deals) == 0 {
			return xerrors.Errorf("no storage deals found for Piece %s", pcid)
		}

		// prefer an unsealed sector containing the piece if one exists
		var isUnsealed bool
		for _, deal := range pinfo.Deals {
			log.Infow("processing deal", "deal_id", deal.DealID, "piece_cid", pcid)

			isUnsealed, err = r.spn.IsUnsealed(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
			if err != nil {
				log.Warnw("failed to check if piece is unsealed; skipping", "piece_cid", pcid, "err", err)
				continue
			}
			if isUnsealed {
				log.Infow("piece is unsealed", "deal_id", deal.DealID, "piece_cid", pcid)
				break
			}
		}

		log.Infow("now registering deal in dag store",
			"deal_id", deal.DealID,
			"is_unsealed", isUnsealed,
			"piece_cid", deal.Ref.PieceCid)

		// Register the deal as a shard with the DAG store, initializing the
		// index immediately if the deal is unsealed (if the deal is not
		// unsealed it will be initialized "lazily" once it's unsealed during
		// retrieval)

		err = r.dagStore.RegisterShard(ctx, *deal.Ref.PieceCid, "", isUnsealed, resch)
		if err != nil {
			log.Warnw("failed to register shard", "deal_id", deal.DealID, "piece_cid", deal.Ref.PieceCid, "error", err)
			continue
		}
		registered++
	}

	log.Infow("finished registering all shards", "total", registered)

	totalCh <- registered

	// Completed registering all shards, so mark the migration as complete
	err = r.markRegistrationComplete()
	if err != nil {
		log.Errorf("failed to mark shards as registered: %s", err)
	} else {
		log.Info("successfully marked migration as complete")
	}

	log.Infow("dagstore migration complete")

	return nil
}

// Check for the existence of a "marker" file indicating that the migration
// has completed
func (r *ShardMigrator) registrationComplete() (bool, error) {
	_, err := os.Stat(r.markerFilePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Create a "marker" file indicating that the migration has completed
func (r *ShardMigrator) markRegistrationComplete() error {
	file, err := os.Create(r.markerFilePath)
	if err != nil {
		return err
	}
	return file.Close()
}
