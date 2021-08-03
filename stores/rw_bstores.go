package stores

import (
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

type rwEntry struct {
	bs   *blockstore.ReadWrite
	path string
}

// ReadWriteBlockstores tracks ReadWrite CAR blockstores.
type ReadWriteBlockstores struct {
	mu     sync.RWMutex
	stores map[string]rwEntry
}

func NewReadWriteBlockstores() *ReadWriteBlockstores {
	return &ReadWriteBlockstores{
		stores: make(map[string]rwEntry),
	}
}

func (r *ReadWriteBlockstores) Get(key string) (*blockstore.ReadWrite, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if e, ok := r.stores[key]; ok {
		return e.bs, e.path, nil
	}
	return nil, "", xerrors.Errorf("could not get blockstore for key %s: %w", key, ErrNotFound)
}

func (r *ReadWriteBlockstores) GetOrCreate(key string, path string, rootCid cid.Cid) (*blockstore.ReadWrite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.stores[key]; ok {
		return e.bs, nil
	}

	bs, err := blockstore.OpenReadWrite(path, []cid.Cid{rootCid}, blockstore.UseWholeCIDs(true))
	if err != nil {
		return nil, xerrors.Errorf("failed to create read-write blockstore: %w", err)
	}
	fmt.Println("************", path)
	r.stores[key] = rwEntry{
		bs:   bs,
		path: path,
	}
	return bs, nil
}

func (r *ReadWriteBlockstores) Untrack(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.stores[key]; ok {
		// If the blockstore has already been finalized, calling Finalize again
		// will return an error. For our purposes it's simplest if Finalize is
		// idempotent so we just ignore any error.
		_ = e.bs.Finalize()
	}

	delete(r.stores, key)
	return nil
}
