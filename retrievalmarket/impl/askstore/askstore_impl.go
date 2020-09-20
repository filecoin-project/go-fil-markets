package askstore

import (
	"bytes"
	"sync"

	"github.com/ipfs/go-datastore"
	"github.com/prometheus/common/log"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
)

// AskStoreImpl implements AskStore, persisting a retrieval Ask
// to disk. It also maintains a cache of the current Ask in memory
type AskStoreImpl struct {
	lk  sync.RWMutex
	ask *retrievalmarket.Ask
	ds  datastore.Batching
	key datastore.Key
}

// NewAskStore returns a new instance of AskStoreImpl
// It will initialize a new default ask and store it if one is not set.
// Otherwise it loads the current Ask from disk
func NewAskStore(ds datastore.Batching, key datastore.Key) (*AskStoreImpl, error) {
	s := &AskStoreImpl{
		ds:  ds,
		key: key,
	}

	if err := s.tryLoadAsk(); err != nil {
		return nil, err
	}

	if s.ask == nil {
		// for now set a default retrieval ask
		defaultAsk := &retrievalmarket.Ask{
			PricePerByte:            retrievalimpl.DefaultPricePerByte,
			UnsealPrice:             big.Zero(),
			PaymentInterval:         retrievalimpl.DefaultPaymentInterval,
			PaymentIntervalIncrease: retrievalimpl.DefaultPaymentIntervalIncrease,
		}

		if err := s.SetAsk(defaultAsk); err != nil {
			return nil, xerrors.Errorf("failed setting a default retrieval ask: %w", err)
		}
	}
	return s, nil
}

// SetAsk stores retrieval provider's ask
func (s *AskStoreImpl) SetAsk(ask *retrievalmarket.Ask) error {
	s.lk.Lock()
	defer s.lk.Unlock()

	return s.saveAsk(ask)
}

// GetAsk returns the current retrieval ask, or nil if one does not exist.
func (s *AskStoreImpl) GetAsk() *retrievalmarket.Ask {
	s.lk.RLock()
	defer s.lk.RUnlock()
	if s.ask == nil {
		return nil
	}
	ask := *s.ask
	return &ask
}

func (s *AskStoreImpl) tryLoadAsk() error {
	s.lk.Lock()
	defer s.lk.Unlock()

	err := s.loadAsk()

	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			log.Warn("no previous ask found, miner will not accept retrieval deals until an ask is set")
			return nil
		}
		return err
	}

	return nil
}

func (s *AskStoreImpl) loadAsk() error {
	askb, err := s.ds.Get(s.key)
	if err != nil {
		return xerrors.Errorf("failed to load most recent retrieval ask from disk: %w", err)
	}

	var ask retrievalmarket.Ask
	if err := cborutil.ReadCborRPC(bytes.NewReader(askb), &ask); err != nil {
		return err
	}

	s.ask = &ask
	return nil
}

func (s *AskStoreImpl) saveAsk(a *retrievalmarket.Ask) error {
	b, err := cborutil.Dump(a)
	if err != nil {
		return err
	}

	if err := s.ds.Put(s.key, b); err != nil {
		return err
	}

	s.ask = a
	return nil
}
