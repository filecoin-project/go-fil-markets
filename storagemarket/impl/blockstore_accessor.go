package storageimpl

import (
	"path/filepath"

	"github.com/filecoin-project/go-fil-markets/stores"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"
)

type carFileBSAccessor struct {
	baseDir   string
	robstores *stores.ReadOnlyBlockstores
}

var _ BlockstoreAccessor = (*carFileBSAccessor)(nil)

func NewCarFileBlockstoreAccessor(baseDir string) *carFileBSAccessor {
	return &carFileBSAccessor{
		baseDir:   baseDir,
		robstores: stores.NewReadOnlyBlockstores(),
	}
}

func (b *carFileBSAccessor) Get(key string) (bstore.Blockstore, error) {
	carPath := b.getPath(key)

	// Open a read-only blockstore off the CAR file, wrapped in a filestore so
	// it can read file positional references.
	bs, err := stores.ReadOnlyFilestore(carPath)
	if err != nil {
		return nil, xerrors.Errorf("failed to open car filestore: %w", err)
	}

	_, err = b.robstores.Track(key, bs)
	if err != nil {
		return nil, xerrors.Errorf("failed to get blockstore from tracker: %w", err)
	}
	return bs, nil
}

func (b *carFileBSAccessor) Close(key string) error {
	return b.robstores.Untrack(key)
}

func (b *carFileBSAccessor) getPath(key string) string {
	return filepath.Join(b.baseDir, key)
}

type passThroughBSAccessor struct {
	bs bstore.Blockstore
}

var _ BlockstoreAccessor = (*passThroughBSAccessor)(nil)

func NewPassThroughBlockstoreAccessor(bs bstore.Blockstore) *passThroughBSAccessor {
	return &passThroughBSAccessor{bs: bs}
}

func (p *passThroughBSAccessor) Get(key string) (bstore.Blockstore, error) {
	return p.bs, nil
}

func (p *passThroughBSAccessor) Close(key string) error {
	return nil
}
