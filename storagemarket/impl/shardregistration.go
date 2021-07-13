package storageimpl

import (
	"context"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
)

var shardRegKey = datastore.NewKey("shards-registered")

type ShardRegistration struct {
	ds       datastore.Datastore
	dagStore dagstore.DagStoreWrapper
}

func NewShardRegistration(ds datastore.Datastore, dagStore dagstore.DagStoreWrapper) *ShardRegistration {
	return &ShardRegistration{
		ds:       ds,
		dagStore: dagStore,
	}
}

func (r *ShardRegistration) registerShards(ctx context.Context, deals []storagemarket.MinerDeal) error {
	// Check if all deals have already been registered as shards
	_, err := r.ds.Get(shardRegKey)
	if err == nil {
		// All shards have been registered, bail out
		return nil
	}
	// Expect ErrNotFound if deals have not been registered as shards
	if !xerrors.Is(err, datastore.ErrNotFound) {
		// There was some other error (not ErrNotFound)
		return xerrors.Errorf("failed to get shard registration status: %w", err)
	}

	// Filter for deals that are currently sealing.
	// If the deal has not yet been handed off to the sealing subsystem, we
	// don't need to call RegisterShard in this migration; RegisterShard will
	// be called in the new code once the deal reaches the state where it's
	// handed off to the sealing subsystem.
	for _, deal := range deals {
		if deal.Ref.PieceCid == nil {
			continue
		}

		// Check if the deal has been handed off to the sealing subsystem
		var sealing bool
		for _, state := range providerstates.ProviderSealingStates {
			if deal.State == state {
				sealing = true
				break
			}
		}
		if !sealing {
			continue
		}

		// Register the deal as a shard with the DAG store
		r.dagStore.RegisterShard(ctx, *deal.Ref.PieceCid, deal.CARv2FilePath)
	}

	// Completed registering all shards, so mark the migration as complete
	err = r.ds.Put(shardRegKey, []byte{1})
	if err != nil {
		log.Errorf("failed to mark shards as registered: %w", err)
	}

	return nil
}
