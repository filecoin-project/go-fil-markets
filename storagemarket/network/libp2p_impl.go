package network

import (
	"bufio"
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/jpillora/backoff"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

const maxStreamOpenAttempts = 5

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

func (impl *libp2pStorageMarketNetwork) NewAskStream(ctx context.Context, id peer.ID) (StorageAskStream, error) {
	s, err := impl.openStream(ctx, id, storagemarket.AskProtocolID)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &askStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) NewDealStream(ctx context.Context, id peer.ID) (StorageDealStream, error) {
	s, err := impl.openStream(ctx, id, storagemarket.DealProtocolID)
	if err != nil {
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &dealStream{p: id, rw: s, buffered: buffered, host: impl.host}, nil
}

func (impl *libp2pStorageMarketNetwork) NewDealStatusStream(ctx context.Context, id peer.ID) (DealStatusStream, error) {
	s, err := impl.openStream(ctx, id, storagemarket.DealStatusProtocolID)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &dealStatusStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) openStream(ctx context.Context, id peer.ID, protocol protocol.ID) (network.Stream, error) {
	b := &backoff.Backoff{
		Min:    1 * time.Second,
		Max:    5 * time.Minute,
		Factor: 5,
		Jitter: true,
	}

	for {
		s, err := impl.host.NewStream(ctx, id, protocol)
		if err == nil {
			return s, err
		}

		nAttempts := b.Attempt()
		if nAttempts == maxStreamOpenAttempts {
			return nil, xerrors.Errorf("exhausted %d attempts but failed to open stream, err: %w", maxStreamOpenAttempts, err)
		}
		d := b.Duration()
		time.Sleep(d)
	}
}

func (impl *libp2pStorageMarketNetwork) SetDelegate(r StorageReceiver) error {
	impl.receiver = r
	impl.host.SetStreamHandler(storagemarket.DealProtocolID, impl.handleNewDealStream)
	impl.host.SetStreamHandler(storagemarket.AskProtocolID, impl.handleNewAskStream)
	impl.host.SetStreamHandler(storagemarket.DealStatusProtocolID, impl.handleNewDealStatusStream)
	return nil
}

func (impl *libp2pStorageMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	impl.host.RemoveStreamHandler(storagemarket.DealProtocolID)
	impl.host.RemoveStreamHandler(storagemarket.AskProtocolID)
	impl.host.RemoveStreamHandler(storagemarket.DealStatusProtocolID)
	return nil
}

func (impl *libp2pStorageMarketNetwork) handleNewAskStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		as := &askStream{s.Conn().RemotePeer(), s, reader}
		impl.receiver.HandleAskStream(as)
	}
}

func (impl *libp2pStorageMarketNetwork) handleNewDealStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		ds := &dealStream{s.Conn().RemotePeer(), impl.host, s, reader}
		impl.receiver.HandleDealStream(ds)
	}
}

func (impl *libp2pStorageMarketNetwork) handleNewDealStatusStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		qs := &dealStatusStream{s.Conn().RemotePeer(), impl.host, s, reader}
		impl.receiver.HandleDealStatusStream(qs)
	}
}

func (impl *libp2pStorageMarketNetwork) getReaderOrReset(s network.Stream) *bufio.Reader {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return nil
	}
	return bufio.NewReaderSize(s, 16)
}

func (impl *libp2pStorageMarketNetwork) ID() peer.ID {
	return impl.host.ID()
}

func (impl *libp2pStorageMarketNetwork) AddAddrs(p peer.ID, addrs []ma.Multiaddr) {
	impl.host.Peerstore().AddAddrs(p, addrs, 8*time.Hour)
}

func (impl *libp2pStorageMarketNetwork) TagPeer(p peer.ID, id string) {
	impl.host.ConnManager().TagPeer(p, id, TagPriority)
}

func (impl *libp2pStorageMarketNetwork) UntagPeer(p peer.ID, id string) {
	impl.host.ConnManager().UntagPeer(p, id)
}
