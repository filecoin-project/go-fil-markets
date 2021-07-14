package storageimpl

import (
	"context"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/specs-storage/storage"

	mktdagstore "github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
)

var shardRegKey = datastore.NewKey("shards-registered")

type SectorStateAccessor interface {
	StateSectorGetInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorOnChainInfo, error)
	IsUnsealed(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error)
}

// ShardMigrator is used to register all deals that are in the sealing / sealed
// state with the DAG store as shards.
// It will only run once on startup, from that point forward deals will be
// registered as shards as part of the deals FSM.
type ShardMigrator struct {
	providerAddr address.Address
	ds           datastore.Datastore
	dagStore     mktdagstore.DagStoreWrapper
	sectorState  SectorStateAccessor
}

func NewShardMigrator(
	maddr address.Address,
	ds datastore.Datastore,
	dagStore mktdagstore.DagStoreWrapper,
	sectorState SectorStateAccessor,
) *ShardMigrator {
	return &ShardMigrator{
		providerAddr: maddr,
		ds:           ds,
		dagStore:     dagStore,
		sectorState:  sectorState,
	}
}

func (r *ShardMigrator) registerShards(ctx context.Context, deals []storagemarket.MinerDeal) error {
	// Check if all deals have already been registered as shards
	has, err := r.ds.Has(shardRegKey)
	if err != nil {
		return xerrors.Errorf("failed to get shard registration status: %w", err)
	}
	if has {
		// All deals have been registered as shards, bail out
		return nil
	}

	// Filter for deals that are currently sealing.
	// If the deal has not yet been handed off to the sealing subsystem, we
	// don't need to call RegisterShard in this migration; RegisterShard will
	// be called in the new code once the deal reaches the state where it's
	// handed off to the sealing subsystem.
	var registered int
	resch := make(chan dagstore.ShardResult, len(deals))
	for _, deal := range deals {
		if deal.Ref.PieceCid == nil {
			continue
		}

		// Check if the deal has been handed off to the sealing subsystem
		var inSealingSubsystem bool
		for _, state := range providerstates.StatesKnownBySealingSubsystem {
			if deal.State == state {
				inSealingSubsystem = true
				break
			}
		}
		if !inSealingSubsystem {
			continue
		}

		// Check if the deal is in an unsealed state
		isUnsealed, err := r.isUnsealed(ctx, deal.SectorNumber)
		if err != nil {
			isUnsealed = false
			log.Errorf("failed to get unsealed state of deal with piece CID %s: %s", deal.Ref.PieceCid, err)
		}

		// Register the deal as a shard with the DAG store, initializing the
		// index immediately if the deal is unsealed (if the deal is not
		// unsealed it will be initialized "lazily" once it's unsealed during
		// retrieval)
		r.dagStore.RegisterShardAsync(ctx, *deal.Ref.PieceCid, deal.CARv2FilePath, isUnsealed, resch)
		registered++
	}

	// If there are any problems registering shards, just log an error
	go func() {
		for i := 0; i < registered; i++ {
			res := <-resch
			if res.Error != nil {
				log.Warnf("dagstore migration: failed to register shard: %s", res.Error)
			}
		}
	}()

	// Completed registering all shards, so mark the migration as complete
	err = r.ds.Put(shardRegKey, []byte{1})
	if err != nil {
		log.Errorf("failed to mark shards as registered: %s", err)
	}

	err = r.ds.Sync(shardRegKey)
	if err != nil {
		log.Errorf("failed to sync shards as registered: %s", err)
	}

	return nil
}

func (r *ShardMigrator) isUnsealed(ctx context.Context, sectorID abi.SectorNumber) (bool, error) {
	// Get the sector seal proof
	secInfo, err := r.sectorState.StateSectorGetInfo(ctx, r.providerAddr, sectorID, types.EmptyTSK)
	if err != nil {
		return false, xerrors.Errorf("failed to get sector %d info: %w", sectorID, err)
	}

	mid, err := address.IDFromAddress(r.providerAddr)
	if err != nil {
		return false, xerrors.Errorf("failed to convert addr %s to ID address: %w", r.providerAddr, err)
	}

	ref := storage.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: sectorID,
		},
		ProofType: secInfo.SealProof,
	}

	// At the time this migration was written all deals in a sector are either
	// sealed or unsealed. It's not possible for there to be a mixture of
	// sealed and unsealed deals in a sector.
	// Therefore the offset and size of the deal in the sector are not
	// important.
	isUnsealed, err := r.sectorState.IsUnsealed(ctx, ref, 0, 1)
	if err != nil {
		return false, xerrors.Errorf("failed to check if sector %d is unsealed: %w", sectorID, err)
	}

	return isUnsealed, nil
}
