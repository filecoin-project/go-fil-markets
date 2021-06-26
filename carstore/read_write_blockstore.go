package carstore

import (
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

// CarReadWriteStoreTracker tracks the lifecycle of a ReadWrite CAR Blockstore to make it easy to create/get/cleanup the blockstores.
// It's important to close a CAR Blockstore when done using it so that the backing CAR file can be closed.
type CarReadWriteStoreTracker struct {
	mu     sync.Mutex
	stores map[string]*blockstore.ReadWrite
}

func NewCarReadWriteStoreTracker() (*CarReadWriteStoreTracker, error) {
	return &CarReadWriteStoreTracker{
		stores: make(map[string]*blockstore.ReadWrite),
	}, nil
}

func (r *CarReadWriteStoreTracker) GetOrCreate(key string, carFilePath string, rootCid cid.Cid) (*blockstore.ReadWrite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	rwBs, err := blockstore.NewReadWrite(carFilePath, []cid.Cid{rootCid})
	if err != nil {
		return nil, err
	}
	r.stores[key] = rwBs

	return rwBs, nil
}

func (r *CarReadWriteStoreTracker) Get(key string) (*blockstore.ReadWrite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	return nil, xerrors.New("not found")
}

func (r *CarReadWriteStoreTracker) Clean(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		delete(r.stores, key)

		// CAR team TODO
		// Should be able to close a ReadOnlyStore.
		// Should be able to close a ReadWriteBlockstore.
		// Finalise/Close should be idempotent.
		// Calling close after finalise shoudln’t return an error.

		// we're cleaning up the blockstore here, so it's okay if finalize fails.
		_ = bs.Finalize()
		return bs.Close()
	}

	return nil
}
