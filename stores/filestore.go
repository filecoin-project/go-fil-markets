package stores

import (
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-filestore"
	"github.com/ipfs/go-ipfs-blockstore"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

// FilestoreOf returns a FileManager/Filestore backed entirely by a
// blockstore without requiring a datastore. It achieves this by coercing the
// blockstore into a datastore. The resulting blockstore is suitable for usage
// with DagBuilderHelper with DagBuilderParams#NoCopy=true.
func FilestoreOf(bs blockstore.Blockstore) (blockstore.Blockstore, error) {
	coercer := &dsCoercer{bs}

	// the FileManager stores positional infos (positional mappings) in a
	// datastore, which in our case is the blockstore coerced into a datastore.
	//
	// Passing the root dir as a base path makes me uneasy, but these filestores
	// are only used locally.
	fm := filestore.NewFileManager(coercer, "/")
	fm.AllowFiles = true

	// the Filestore sifts leaves (PosInfos) from intermediate nodes. It writes
	// PosInfo leaves to the datastore (which in our case is the coerced
	// blockstore), and the intermediate nodes to the blockstore proper (since
	// they cannot be mapped to the file.
	fstore := filestore.NewFilestore(bs, fm)
	bs = blockstore.NewIdStore(fstore)

	return bs, nil
}

var cidBuilder = cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}

// dsCoercer coerces a Blockstore to present a datastore interface, apt for
// usage with the Filestore/FileManager. Only PosInfos will be written through
// this path.
type dsCoercer struct {
	blockstore.Blockstore
}

var _ datastore.Batching = (*dsCoercer)(nil)

func (crcr *dsCoercer) Get(key datastore.Key) (value []byte, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to create cid: %w", err)
	}

	blk, err := crcr.Blockstore.Get(c)
	if err != nil {
		return nil, xerrors.Errorf("failed to get cid %s: %w", c, err)
	}
	return blk.RawData(), nil
}

func (crcr *dsCoercer) Put(key datastore.Key, value []byte) error {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to create cid: %w", err)
	}
	blk, err := blocks.NewBlockWithCid(value, c)
	if err != nil {
		return xerrors.Errorf("failed to create block: %w", err)
	}
	if err := crcr.Blockstore.Put(blk); err != nil {
		return xerrors.Errorf("failed to put block: %w", err)
	}
	return nil
}

func (crcr *dsCoercer) Has(key datastore.Key) (exists bool, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return false, xerrors.Errorf("failed to create cid: %w", err)
	}
	return crcr.Blockstore.Has(c)
}

func (crcr *dsCoercer) Batch() (datastore.Batch, error) {
	return datastore.NewBasicBatch(crcr), nil
}

func (crcr *dsCoercer) GetSize(_ datastore.Key) (size int, err error) {
	return 0, xerrors.New("operation NOT supported: GetSize")
}

func (crcr *dsCoercer) Query(_ query.Query) (query.Results, error) {
	return nil, xerrors.New("operation NOT supported: Query")
}

func (crcr *dsCoercer) Delete(_ datastore.Key) error {
	return xerrors.New("operation NOT supported: Delete")
}

func (crcr *dsCoercer) Sync(_ datastore.Key) error {
	return xerrors.New("operation NOT supported: Sync")
}

func (crcr *dsCoercer) Close() error {
	return nil
}
