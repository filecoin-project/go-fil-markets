package discovery

//go:generate cbor-gen-for retrievalPeers

import (
	"bytes"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dshelp "github.com/ipfs/go-ipfs-ds-help"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type Local struct {
	ds datastore.Datastore
}

// convenience struct for encoding slices of RetrievalPeer
type retrievalPeers struct {
	Peers []retrievalmarket.RetrievalPeer
}

func NewLocal(ds datastore.Batching) *Local {
	return &Local{ds: ds}
}

func (l *Local) AddPeer(cid cid.Cid, peer retrievalmarket.RetrievalPeer) error {
	key := dshelp.MultihashToDsKey(cid.Hash())
	exists, err := l.ds.Has(key)
	if err != nil {
		return err
	}

	var newRecord bytes.Buffer

	if !exists {
		peers := retrievalPeers{Peers: []retrievalmarket.RetrievalPeer{peer}}
		err = peers.MarshalCBOR(&newRecord)
		if err != nil {
			return err
		}
	} else {
		entry, err := l.ds.Get(key)
		if err != nil {
			return err
		}
		var peers retrievalPeers
		if err = peers.UnmarshalCBOR(bytes.NewReader(entry)); err != nil {
			return err
		}
		if hasPeer(peers, peer) {
			return nil
		}
		peers.Peers = append(peers.Peers, peer)
		err = peers.MarshalCBOR(&newRecord)
		if err != nil {
			return err
		}
	}

	return l.ds.Put(key, newRecord.Bytes())
}

func hasPeer(peerList retrievalPeers, peer retrievalmarket.RetrievalPeer) bool {
	for _, p := range peerList.Peers {
		if p == peer {
			return true
		}
	}
	return false
}

func (l *Local) GetPeers(payloadCID cid.Cid) ([]retrievalmarket.RetrievalPeer, error) {
	entry, err := l.ds.Get(dshelp.MultihashToDsKey(payloadCID.Hash()))
	if err == datastore.ErrNotFound {
		return []retrievalmarket.RetrievalPeer{}, nil
	}
	if err != nil {
		return nil, err
	}
	var peers retrievalPeers
	if err := peers.UnmarshalCBOR(bytes.NewReader(entry)); err != nil {
		return nil, err
	}
	return peers.Peers, nil
}

var _ retrievalmarket.PeerResolver = &Local{}
