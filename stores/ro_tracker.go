package stores

import (
	"io"
	"sync"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

type ClosableBlockstore interface {
	bstore.Blockstore
	io.Closer
}

// CarReadOnlyStoreTracker tracks the lifecycle of a ReadOnly CAR Blockstore and makes it easy to create/get/cleanup the blockstores.
// It's important to close a CAR Blockstore when done using it so that the backing CAR file can be closed.
type CarReadOnlyStoreTracker struct {
	mu     sync.RWMutex
	stores map[string]bstore.Blockstore
}

func NewReadOnlyStoreTracker() *CarReadOnlyStoreTracker {
	return &CarReadOnlyStoreTracker{
		stores: make(map[string]bstore.Blockstore),
	}
}

func (r *CarReadOnlyStoreTracker) Add(key string, bs bstore.Blockstore) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.stores[key]; ok {
		return false, nil
	}

	r.stores[key] = bs
	return true, nil
}

func (r *CarReadOnlyStoreTracker) GetOrCreate(key string, carFilePath string) (bstore.Blockstore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	rdOnly, err := blockstore.OpenReadOnly(carFilePath, carv2.ZeroLengthSectionAsEOF(true), blockstore.UseWholeCIDs(true))
	if err != nil {
		return nil, xerrors.Errorf("failed to open read-only blockstore: %w", err)
	}
	r.stores[key] = rdOnly

	return rdOnly, nil
}

func (r *CarReadOnlyStoreTracker) Get(key string) (bstore.Blockstore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	return nil, xerrors.Errorf("could not get blockstore for key %s: %w", key, ErrNotFound)
}

func (r *CarReadOnlyStoreTracker) CleanBlockstore(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		delete(r.stores, key)
		if closer, ok := bs.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				return xerrors.Errorf("failed to close read-only blockstore: %w", err)
			}
		}
	}

	return nil
}
