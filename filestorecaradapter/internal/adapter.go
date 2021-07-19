package internal

import (
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

var cidBuilder = cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}

var _ datastore.Batching = (*bsToDSBatchingAdapter)(nil)

// transforms a blockstore interface to a datastore.Batching interface.
type bsToDSBatchingAdapter struct {
	bstore.Blockstore
}

func BlockstoreToDSBatchingAdapter(bs bstore.Blockstore) *bsToDSBatchingAdapter {
	return &bsToDSBatchingAdapter{bs}
}

func (a *bsToDSBatchingAdapter) Get(key datastore.Key) (value []byte, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to create cid: %w", err)
	}

	blk, err := a.Blockstore.Get(c)
	if err != nil {
		return nil, xerrors.Errorf("failed to get cid %s: %w", c, err)
	}
	return blk.RawData(), nil
}

func (a *bsToDSBatchingAdapter) Put(key datastore.Key, value []byte) error {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to create cid: %w", err)
	}

	blk, err := blocks.NewBlockWithCid(value, c)
	if err != nil {
		return xerrors.Errorf("failed to create block: %w", err)
	}

	if err := a.Blockstore.Put(blk); err != nil {
		return xerrors.Errorf("failed to put block: %w", err)
	}

	return nil
}

func (a *bsToDSBatchingAdapter) Has(key datastore.Key) (exists bool, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return false, xerrors.Errorf("failed to create cid: %w", err)

	}

	return a.Blockstore.Has(c)
}

func (a *bsToDSBatchingAdapter) Batch() (datastore.Batch, error) {
	return datastore.NewBasicBatch(a), nil
}

func (a *bsToDSBatchingAdapter) GetSize(_ datastore.Key) (size int, err error) {
	return 0, xerrors.New("operation NOT supported: GetSize")
}

func (a *bsToDSBatchingAdapter) Query(_ query.Query) (query.Results, error) {
	return nil, xerrors.New("operation NOT supported: Query")
}

func (a *bsToDSBatchingAdapter) Delete(_ datastore.Key) error {
	return xerrors.New("operation NOT supported: Delete")
}

func (a *bsToDSBatchingAdapter) Sync(_ datastore.Key) error {
	return xerrors.New("operation NOT supported: Sync")
}

func (a *bsToDSBatchingAdapter) Close() error {
	return nil
}
