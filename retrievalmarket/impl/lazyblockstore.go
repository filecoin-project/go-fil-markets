package retrievalimpl

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

type lazyBlockstore struct {
	lk   sync.Mutex
	bs   bstore.Blockstore
	load func() (bstore.Blockstore, error)
}

func newLazyBlockstore(load func() (bstore.Blockstore, error)) *lazyBlockstore {
	return &lazyBlockstore{
		load: load,
	}
}

func (l *lazyBlockstore) DeleteBlock(c cid.Cid) error {
	bs, err := l.init()
	if err != nil {
		return err
	}
	return bs.DeleteBlock(c)
}

func (l *lazyBlockstore) Has(c cid.Cid) (bool, error) {
	bs, err := l.init()
	if err != nil {
		return false, err
	}
	return bs.Has(c)
}

func (l *lazyBlockstore) Get(c cid.Cid) (blocks.Block, error) {
	bs, err := l.init()
	if err != nil {
		return nil, err
	}
	return bs.Get(c)
}

func (l *lazyBlockstore) GetSize(c cid.Cid) (int, error) {
	bs, err := l.init()
	if err != nil {
		return 0, err
	}
	return bs.GetSize(c)
}

func (l *lazyBlockstore) Put(block blocks.Block) error {
	bs, err := l.init()
	if err != nil {
		return err
	}
	return bs.Put(block)
}

func (l *lazyBlockstore) PutMany(blocks []blocks.Block) error {
	bs, err := l.init()
	if err != nil {
		return err
	}
	return bs.PutMany(blocks)
}

func (l *lazyBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	bs, err := l.init()
	if err != nil {
		return nil, err
	}
	return bs.AllKeysChan(ctx)
}

func (l *lazyBlockstore) HashOnRead(enabled bool) {
	bs, err := l.init()
	if err != nil {
		return
	}
	bs.HashOnRead(enabled)
}

func (l *lazyBlockstore) init() (bstore.Blockstore, error) {
	l.lk.Lock()
	defer l.lk.Unlock()

	if l.bs == nil {
		var err error
		l.bs, err = l.load()
		if err != nil {
			return nil, err
		}
	}
	return l.bs, nil
}

var _ bstore.Blockstore = (*lazyBlockstore)(nil)
