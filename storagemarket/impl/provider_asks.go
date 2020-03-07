package storageimpl

import (
	"bytes"
	"context"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

func (p *Provider) AddAsk(price abi.TokenAmount, duration abi.ChainEpoch) error {
	p.askLk.Lock()
	defer p.askLk.Unlock()

	var seqno uint64
	if p.ask != nil {
		seqno = p.ask.Ask.SeqNo + 1
	}

	stateKey, err := p.spn.MostRecentStateId(context.TODO())
	if err != nil {
		return err
	}
	ask := &storagemarket.StorageAsk{
		Price:        price,
		Timestamp:    stateKey.Height(),
		Expiry:       stateKey.Height() + duration,
		Miner:        p.actor,
		SeqNo:        seqno,
		MinPieceSize: p.minPieceSize,
	}

	ssa, err := p.signAsk(ask)
	if err != nil {
		return err
	}

	return p.saveAsk(ssa)
}

func (p *Provider) GetAsk(m address.Address) *storagemarket.SignedStorageAsk {
	p.askLk.Lock()
	defer p.askLk.Unlock()
	if m != p.actor {
		return nil
	}

	return p.ask
}

func (p *Provider) HandleAskStream(s network.StorageAskStream) {
	defer s.Close()
	ar, err := s.ReadAskRequest()
	if err != nil {
		log.Errorf("failed to read AskRequest from incoming stream: %s", err)
		return
	}

	resp := p.processAskRequest(&ar)

	if err := s.WriteAskResponse(resp); err != nil {
		log.Errorf("failed to write ask response: %s", err)
		return
	}
}

func (p *Provider) processAskRequest(ar *network.AskRequest) network.AskResponse {
	return network.AskResponse{
		Ask: p.GetAsk(ar.Miner),
	}
}

var bestAskKey = datastore.NewKey("latest-ask")

func (p *Provider) tryLoadAsk() error {
	p.askLk.Lock()
	defer p.askLk.Unlock()

	err := p.loadAsk()
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			log.Warn("no previous ask found, miner will not accept deals until a price is set")
			return nil
		}
		return err
	}

	return nil
}

func (p *Provider) loadAsk() error {
	askb, err := p.ds.Get(datastore.NewKey("latest-ask"))
	if err != nil {
		return xerrors.Errorf("failed to load most recent ask from disk: %w", err)
	}

	var ssa storagemarket.SignedStorageAsk
	if err := cborutil.ReadCborRPC(bytes.NewReader(askb), &ssa); err != nil {
		return err
	}

	p.ask = &ssa
	return nil
}

func (p *Provider) signAsk(a *storagemarket.StorageAsk) (*storagemarket.SignedStorageAsk, error) {
	b, err := cborutil.Dump(a)
	if err != nil {
		return nil, err
	}

	worker, err := p.spn.GetMinerWorker(context.TODO(), p.actor)
	if err != nil {
		return nil, xerrors.Errorf("failed to get worker to sign ask: %w", err)
	}

	sig, err := p.spn.SignBytes(context.TODO(), worker, b)
	if err != nil {
		return nil, err
	}

	return &storagemarket.SignedStorageAsk{
		Ask:       a,
		Signature: sig,
	}, nil
}

func (p *Provider) saveAsk(a *storagemarket.SignedStorageAsk) error {
	b, err := cborutil.Dump(a)
	if err != nil {
		return err
	}

	if err := p.ds.Put(bestAskKey, b); err != nil {
		return err
	}

	p.ask = a
	return nil
}
