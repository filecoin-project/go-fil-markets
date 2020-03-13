package network

import (
	"bufio"
	"context"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

var log = logging.Logger("storagemarket_network")

// NewFromLibp2pHost builds a storage market network on top of libp2p
func NewFromLibp2pHost(h host.Host) StorageMarketNetwork {
	return &libp2pStorageMarketNetwork{host: h}
}

// libp2pStorageMarketNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
type libp2pStorageMarketNetwork struct {
	host host.Host
	// inbound messages from the network are forwarded to the receiver
	receiver StorageReceiver
}

func (impl *libp2pStorageMarketNetwork) NewAskStream(id peer.ID) (StorageAskStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, storagemarket.AskProtocolID)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &askStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) NewDealStream(id peer.ID) (StorageDealStream, error) {
	s, err := impl.host.NewStream(context.Background(), id, storagemarket.DealProtocolID)
	if err != nil {
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &dealStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) SetDelegate(r StorageReceiver) error {
	impl.receiver = r
	impl.host.SetStreamHandler(storagemarket.DealProtocolID, impl.handleNewDealStream)
	impl.host.SetStreamHandler(storagemarket.AskProtocolID, impl.handleNewAskStream)
	return nil
}

func (impl *libp2pStorageMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	impl.host.RemoveStreamHandler(storagemarket.DealProtocolID)
	impl.host.RemoveStreamHandler(storagemarket.AskProtocolID)
	return nil
}

func (impl *libp2pStorageMarketNetwork) handleNewAskStream(s network.Stream) {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return
	}
	remotePID := s.Conn().RemotePeer()
	buffered := bufio.NewReaderSize(s, 16)
	as := &askStream{remotePID, s, buffered}
	impl.receiver.HandleAskStream(as)
}

func (impl *libp2pStorageMarketNetwork) handleNewDealStream(s network.Stream) {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return
	}
	remotePID := s.Conn().RemotePeer()
	buffered := bufio.NewReaderSize(s, 16)
	ds := &dealStream{remotePID, s, buffered}
	impl.receiver.HandleDealStream(ds)
}
