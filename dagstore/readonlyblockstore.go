package dagstore

import (
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/dagstore"
)

// ReadOnlyBlockstore stubs out Blockstore mutators with methods that panic
type ReadOnlyBlockstore struct {
	dagstore.ReadBlockstore
}

func NewReadOnlyBlockstore(rbs dagstore.ReadBlockstore) bstore.Blockstore {
	return ReadOnlyBlockstore{ReadBlockstore: rbs}
}

func (r ReadOnlyBlockstore) DeleteBlock(c cid.Cid) error {
	panic("cannot call DeleteBlock on a read-only blockstore")
}

func (r ReadOnlyBlockstore) Put(block blocks.Block) error {
	panic("cannot call Put on a read-only blockstore")
}

func (r ReadOnlyBlockstore) PutMany(blocks []blocks.Block) error {
	panic("cannot call PutMany on a read-only blockstore")
}

var _ bstore.Blockstore = (*ReadOnlyBlockstore)(nil)
