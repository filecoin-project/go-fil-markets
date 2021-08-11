package retrievalimpl

import (
	"path/filepath"

	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

type carFileBSAccessor struct {
	baseDir   string
	rwbstores *stores.ReadWriteBlockstores
}

var _ BlockstoreAccessor = (*carFileBSAccessor)(nil)

func NewCarFileBlockstoreAccessor(baseDir string) *carFileBSAccessor {
	return &carFileBSAccessor{
		baseDir:   baseDir,
		rwbstores: stores.NewReadWriteBlockstores(),
	}
}

func (b *carFileBSAccessor) Get(key string, rootCid cid.Cid) (bstore.Blockstore, error) {
	carPath := b.getPath(key)
	return b.rwbstores.GetOrOpen(key, carPath, rootCid)
}

func (b *carFileBSAccessor) Close(key string) error {
	return b.rwbstores.Untrack(key)
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

func (p *passThroughBSAccessor) Get(key string, rootCid cid.Cid) (bstore.Blockstore, error) {
	return p.bs, nil
}

func (p *passThroughBSAccessor) Close(key string) error {
	return nil
}
