package carstore

import (
	"sync"

	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

// CarReadOnlyStoreTracker tracks the lifecycle of a ReadOnly CAR Blockstore and makes it easy to create/get/cleanup the blockstores.
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

func (r *CarReadOnlyStoreTracker) Add(key string, bs *blockstore.ReadOnly) (bool, error) {
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
		return nil, xerrors.Errorf("failed to open read-only blockstore, err=%w", err)
	}
	r.stores[key] = rdOnly

	return rdOnly, nil
}

func (r *CarReadOnlyStoreTracker) Get(key string) (*blockstore.ReadOnly, error) {
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
		if err := bs.Close(); err != nil {
			return xerrors.Errorf("failed to close read-only blockstore, err=%w", err)
		}
	}

	return nil
}
