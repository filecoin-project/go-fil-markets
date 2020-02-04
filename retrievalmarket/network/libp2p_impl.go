package network

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("retrieval_network")

func NewFromLibp2pHost(h host.Host) RetrievalMarketNetwork {
	return &libp2pRetrievalMarketNetwork{host: h}
}

// libp2pRetrievalMarketNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
type libp2pRetrievalMarketNetwork struct {
	host host.Host
	// inbound messages from the network are forwarded to the receiver
	receiver RetrievalReceiver
}

func (impl *libp2pRetrievalMarketNetwork) NewQueryStream(id peer.ID) (RetrievalQueryStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, retrievalmarket.QueryProtocolID)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	return &QueryStream{p: id, rw: s}, nil
}

func (impl *libp2pRetrievalMarketNetwork) NewDealStream(id peer.ID) (RetrievalDealStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, retrievalmarket.ProtocolID)
	if err != nil {
		return nil, err
	}
	return &DealStream{p: id, rw: s}, nil
}

func (impl *libp2pRetrievalMarketNetwork) SetDelegate(r RetrievalReceiver) error {
	impl.receiver = r
	impl.host.SetStreamHandler(retrievalmarket.ProtocolID, impl.handleNewDealStream)
	impl.host.SetStreamHandler(retrievalmarket.QueryProtocolID, impl.handleNewQueryStream)
	return nil
}

func (impl *libp2pRetrievalMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	impl.host.RemoveStreamHandler(retrievalmarket.ProtocolID)
	impl.host.RemoveStreamHandler(retrievalmarket.QueryProtocolID)
	return nil
}

func (impl *libp2pRetrievalMarketNetwork) handleNewQueryStream(s network.Stream) {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return
	}
	remotePID := s.Conn().RemotePeer()
	qs := &QueryStream{remotePID, s}
	impl.receiver.HandleQueryStream(qs)
}

func (impl *libp2pRetrievalMarketNetwork) handleNewDealStream(s network.Stream) {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return
	}
	remotePID := s.Conn().RemotePeer()
	ds := &DealStream{remotePID, s}
	impl.receiver.HandleDealStream(ds)
}
