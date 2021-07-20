package shared

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/carstore"
)

// DagStoreWrapper hides the details of the DAG store implementation from
// the other parts of go-fil-markets
type DagStoreWrapper interface {
	// RegisterShard loads a CAR file into the DAG store and builds an
	// index for it, sending the result on the supplied channel on completion
	RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error
	// LoadShard fetches the data for a shard and provides a blockstore interface to it
	LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error)
	// Close closes the dag store wrapper.
	Close() error
}

// RegisterShardSync calls the DAGStore RegisterShard method and waits
// synchronously in a dedicated channel until the registration has completed
// fully.
func RegisterShardSync(ctx context.Context, ds DagStoreWrapper, pieceCid cid.Cid, carPath string, eagerInit bool) error {
	resch := make(chan dagstore.ShardResult, 1)
	if err := ds.RegisterShard(ctx, pieceCid, carPath, eagerInit, resch); err != nil {
		return err
	}

	// TODO: Can I rely on RegisterShard to return an error if the context times out?
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-resch:
		return res.Error
	}
}
