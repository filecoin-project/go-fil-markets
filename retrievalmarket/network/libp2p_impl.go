package network

import (
	"context"
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("retrieval_network")

func NewFromLibp2pHost(h host.Host) RetrievalMarketNetwork {
	return libp2pRetrievalMarketNetwork{host: h}
}

// libp2pRetrievalMarketNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
type libp2pRetrievalMarketNetwork struct {
	host host.Host
	// inbound messages from the network are forwarded to the receiver
	receiver RetrievalReceiver
}

func (impl libp2pRetrievalMarketNetwork) NewQueryStream(id peer.ID) (RetrievalQueryStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, retrievalmarket.QueryProtocolID)
	if err != nil {
		return nil, err
	}
	return &QueryStream{p: id, rw: s}, nil
}

func (impl libp2pRetrievalMarketNetwork) NewDealStream(id peer.ID) (RetrievalDealStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, retrievalmarket.ProtocolID)
	if err != nil {
		return nil, err
	}
	return &DealStream{p: id, rw: s}, nil
}

func (impl libp2pRetrievalMarketNetwork) SetDelegate(r RetrievalReceiver) error {
	impl.receiver = r
	impl.host.SetStreamHandler(retrievalmarket.ProtocolID, impl.handleNewStream)
	impl.host.SetStreamHandler(retrievalmarket.QueryProtocolID, impl.handleNewStream)
	return nil
}

func (impl libp2pRetrievalMarketNetwork) handleNewStream(s network.Stream) {
	if impl.receiver == nil {
		return
	}
	remotePID := s.Conn().RemotePeer()
	if s.Protocol() ==  retrievalmarket.QueryProtocolID {
		qs := &QueryStream{remotePID, s}
		impl.receiver.HandleQueryStream(qs)
	} else if s.Protocol() == retrievalmarket.ProtocolID {
		ds := &DealStream{remotePID, s}
		impl.receiver.HandleDealStream(ds)
	}
}

