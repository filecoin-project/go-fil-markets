package askstore

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedds "github.com/filecoin-project/go-ds-versioning/pkg/datastore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
)

// AskStoreAPI defines the API needed by the Ask Store
type AskStoreAPI interface {
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)
	// returns the worker address associated with a miner
	GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error)
	SignBytes(context.Context, address.Address, []byte) (*crypto.Signature, error)
}

// AskStoreImpl implements AskStore, persisting a retrieval Ask
// to disk. It also maintains a cache of the current Ask in memory
type AskStoreImpl struct {
	lk    sync.RWMutex
	ask   *retrievalmarket.SignedRetrievalAsk
	ds    datastore.Batching
	key   datastore.Key
	api   AskStoreAPI
	actor address.Address
}

var _ retrievalmarket.AskStore = (*AskStoreImpl)(nil)

// NewAskStore returns a new instance of AskStoreImpl
// It will initialize a new default ask and store it if one is not set.
// Otherwise it loads the current Ask from disk
func NewAskStore(ds datastore.Batching, key datastore.Key, api AskStoreAPI, actor address.Address) (*AskStoreImpl, error) {
	askMigrations, err := migrations.AskMigrations.Build()
	if err != nil {
		return nil, err
	}
	versionedDs, migrateDs := versionedds.NewVersionedDatastore(ds, askMigrations, versioning.VersionKey("1"))
	err = migrateDs(context.TODO())
	if err != nil {
		return nil, err
	}
	s := &AskStoreImpl{
		ds:    versionedDs,
		key:   key,
		api:   api,
		actor: actor,
	}

	if err := s.tryLoadAsk(); err != nil {
		return nil, err
	}

	if s.ask == nil {
		// for now set a default retrieval ask
		defaultAsk := &retrievalmarket.Ask{
			PricePerByte:            retrievalmarket.DefaultPricePerByte,
			UnsealPrice:             retrievalmarket.DefaultUnsealPrice,
			PaymentInterval:         retrievalmarket.DefaultPaymentInterval,
			PaymentIntervalIncrease: retrievalmarket.DefaultPaymentIntervalIncrease,
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
	return ask.Ask
}

// GetSignedAsk returns the current retrieval ask, signed with the provider's
// key, or nil if there is no ask set.
func (s *AskStoreImpl) GetSignedAsk() (*retrievalmarket.SignedRetrievalAsk, error) {
	s.lk.RLock()
	defer s.lk.RUnlock()
	if s.ask == nil {
		return nil, nil
	}

	// If the ask is not yet signed, sign the ask with the provider worker key
	if s.ask.Signature == nil {
		// Create a context with a short timeout to use when generating the
		// signature
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tok, _, err := s.api.GetChainHead(ctx)
		if err != nil {
			return nil, err
		}

		sig, err := providerutils.SignMinerData(ctx, s.ask.Ask, s.actor, tok, s.api.GetMinerWorkerAddress, s.api.SignBytes)
		if err != nil {
			return nil, err
		}

		s.ask.Signature = sig
	}

	ask := *s.ask
	return &ask, nil
}

func (s *AskStoreImpl) tryLoadAsk() error {
	s.lk.Lock()
	defer s.lk.Unlock()

	err := s.loadAsk()

	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			// this is expected
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

	s.ask = &retrievalmarket.SignedRetrievalAsk{Ask: &ask}
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

	s.ask = &retrievalmarket.SignedRetrievalAsk{Ask: a}
	return nil
}
