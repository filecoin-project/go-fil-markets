package discovery

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/boost-gfm/retrievalmarket"
)

//go:generate cbor-gen-for --map-encoding RetrievalPeers

// RetrievalPeers is a convenience struct for encoding slices of RetrievalPeer
type RetrievalPeers struct {
	Peers []retrievalmarket.RetrievalPeer
}

// PeerResolver is an interface for looking up providers that may have a piece
type PeerResolver interface {
	GetPeers(payloadCID cid.Cid) ([]retrievalmarket.RetrievalPeer, error) // TODO: channel
}
