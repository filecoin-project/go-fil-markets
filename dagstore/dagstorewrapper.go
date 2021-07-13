package dagstore

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/dagstore/shard"

	"github.com/filecoin-project/go-fil-markets/carstore"
)

// DagStoreWrapper hides the details of the DAG store implementation from
// the other parts of go-fil-markets
type DagStoreWrapper interface {
	// RegisterShard loads a CAR file into the DAG store and builds an index for it
	RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string) error
	// LoadShard fetches the data for a shard and provides a blockstore interface to it
	LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error)
}

type dagStoreWrapper struct {
	dagStore *dagstore.DAGStore
	mountApi LotusMountAPI
}

func NewDagStoreWrapper(dsRegistry *mount.Registry, dagStore *dagstore.DAGStore, mountApi LotusMountAPI) (*dagStoreWrapper, error) {
	err := dsRegistry.Register(lotusScheme, NewLotusMountTemplate(mountApi))
	if err != nil {
		return nil, err
	}

	return &dagStoreWrapper{
		dagStore: dagStore,
		mountApi: mountApi,
	}, nil
}

type closableBlockstore struct {
	bstore.Blockstore
	io.Closer
}

func (ds *dagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	key := shard.KeyFromCID(pieceCid)
	resch := make(chan dagstore.ShardResult, 1)
	err := ds.dagStore.AcquireShard(ctx, key, resch, dagstore.AcquireOpts{})
	if err != nil {
		return nil, xerrors.Errorf("failed to schedule acquire shard for piece CID %s: %w", pieceCid, err)
	}

	// TODO: Can I rely on AcquireShard to return an error if the context times out?
	//select {
	//case <-ctx.Done():
	//	return ctx.Err()
	//case res := <-resch:
	//	return nil, res.Error
	//}

	res := <-resch
	if res.Error != nil {
		return nil, xerrors.Errorf("failed to acquire shard for piece CID %s: %w", pieceCid, err)
	}

	bs, err := res.Accessor.Blockstore()
	if err != nil {
		return nil, err
	}

	return &closableBlockstore{Blockstore: NewReadOnlyBlockstore(bs), Closer: res.Accessor}, nil
}

func (ds *dagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string) error {
	key := shard.KeyFromCID(pieceCid)
	mt, err := NewLotusMount(pieceCid, ds.mountApi)
	if err != nil {
		return xerrors.Errorf("failed to create lotus mount for piece CID %s: %w", pieceCid, err)
	}

	opts := dagstore.RegisterOpts{ExistingTransient: carPath}
	resch := make(chan dagstore.ShardResult, 1)
	err = ds.dagStore.RegisterShard(ctx, key, mt, resch, opts)
	if err != nil {
		return xerrors.Errorf("failed to schedule register shard for piece CID %s: %w", pieceCid, err)
	}

	// TODO: Can I rely on RegisterShard to return an error if the context times out?
	//select {
	//case <-ctx.Done():
	//	return ctx.Err()
	//case res := <-resch:
	//	return res.Error
	//}

	res := <-resch
	if res.Error != nil {
		return xerrors.Errorf("failed to register shard for piece CID %s: %w", pieceCid, res.Error)
	}
	return nil
}
