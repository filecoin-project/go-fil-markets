package discovery

import (
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(retrievalmarket.RetrievalPeer{})
}

func Multi(r retrievalmarket.PeerResolver) retrievalmarket.PeerResolver { // TODO: actually support multiple mechanisms
	return r
}
