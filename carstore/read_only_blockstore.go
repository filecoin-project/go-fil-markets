package carstore

import (
	"sync"

	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

// CarReadOnlyStoreTracker tracks the lifecycle of a ReadOnly CAR Blockstore to make it easy to create/get/cleanup the blockstores.
// It's important to close a CAR Blockstore when done using it so that the backing CAR file can be closed.
type CarReadOnlyStoreTracker struct {
	mu     sync.Mutex
	stores map[string]*blockstore.ReadOnly
}

func NewReadOnlyStoreTracker() (*CarReadOnlyStoreTracker, error) {
	return &CarReadOnlyStoreTracker{
		stores: make(map[string]*blockstore.ReadOnly),
	}, nil
}

func (r *CarReadOnlyStoreTracker) AddBlockStore(key string, bs *blockstore.ReadOnly) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.stores[key]; ok {
		return false, nil
	}

	r.stores[key] = bs
	return true, nil
}

func (r *CarReadOnlyStoreTracker) GetOrCreate(key string, carFilePath string) (*blockstore.ReadOnly, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	rdOnly, err := blockstore.OpenReadOnly(carFilePath, true)
	if err != nil {
		return nil, err
	}
	r.stores[key] = rdOnly

	return rdOnly, nil
}

func (r *CarReadOnlyStoreTracker) GetBlockStore(key string) (*blockstore.ReadOnly, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	return nil, xerrors.New("not found")
}

func (r *CarReadOnlyStoreTracker) CleanBlockStore(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		delete(r.stores, key)
		return bs.Close()
	}

	return nil
}
