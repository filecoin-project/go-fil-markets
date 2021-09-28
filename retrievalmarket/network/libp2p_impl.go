package network

import (
	"bufio"
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("retrieval_network")
var _ RetrievalMarketNetwork = new(libp2pRetrievalMarketNetwork)

// Option is an option for configuring the libp2p storage market network
type Option func(*libp2pRetrievalMarketNetwork)

// RetryParameters changes the default parameters around connection reopening
func RetryParameters(minDuration time.Duration, maxDuration time.Duration, attempts float64, backoffFactor float64) Option {
	return func(impl *libp2pRetrievalMarketNetwork) {
		impl.retryStream.SetOptions(shared.RetryParameters(minDuration, maxDuration, attempts, backoffFactor))
	}
}

// SupportedProtocols sets what protocols this network instances listens on
func SupportedProtocols(supportedProtocols []protocol.ID) Option {
	return func(impl *libp2pRetrievalMarketNetwork) {
		impl.supportedQueryProtocols = supportedProtocols
	}
}

// NewFromLibp2pHost constructs a new instance of the RetrievalMarketNetwork from a
// libp2p host
func NewFromLibp2pHost(h host.Host, options ...Option) RetrievalMarketNetwork {
	impl := &libp2pRetrievalMarketNetwork{
		host:                  h,
		retryStream:           shared.NewRetryStream(h),
		supportedAskProtocols: []protocol.ID{retrievalmarket.AskProtocolID},
		supportedQueryProtocols: []protocol.ID{
			retrievalmarket.QueryProtocolID,
			retrievalmarket.OldQueryProtocolID,
		},
	}
	for _, option := range options {
		option(impl)
	}
	return impl
}

// libp2pRetrievalMarketNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
// It implements the RetrievalMarketNetwork API.
type libp2pRetrievalMarketNetwork struct {
	host        host.Host
	retryStream *shared.RetryStream
	// inbound messages from the network are forwarded to the receiver
	receiver                RetrievalReceiver
	supportedAskProtocols   []protocol.ID
	supportedQueryProtocols []protocol.ID
}

// NewAskStream creates a new stream on the ask protocol to the given peer
func (impl *libp2pRetrievalMarketNetwork) NewAskStream(ctx context.Context, id peer.ID) (RetrievalAskStream, error) {
	s, err := impl.retryStream.OpenStream(ctx, id, impl.supportedAskProtocols)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	return &askStream{p: id, rw: s, buffered: buffered}, nil
}

//  NewQueryStream creates a new RetrievalQueryStream using the provided peer.ID
func (impl *libp2pRetrievalMarketNetwork) NewQueryStream(id peer.ID) (RetrievalQueryStream, error) {
	s, err := impl.retryStream.OpenStream(context.Background(), id, impl.supportedQueryProtocols)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	if s.Protocol() == retrievalmarket.OldQueryProtocolID {
		return &oldQueryStream{p: id, rw: s, buffered: buffered}, nil
	}
	return &queryStream{p: id, rw: s, buffered: buffered}, nil
}

// SetDelegate sets a RetrievalReceiver to handle stream data
func (impl *libp2pRetrievalMarketNetwork) SetDelegate(r RetrievalReceiver) error {
	impl.receiver = r
	for _, proto := range impl.supportedAskProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewAskStream)
	}
	for _, proto := range impl.supportedQueryProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewQueryStream)
	}
	return nil
}

// StopHandlingRequests unsets the RetrievalReceiver and would perform any other necessary
// shutdown logic.
func (impl *libp2pRetrievalMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	for _, proto := range impl.supportedAskProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	for _, proto := range impl.supportedQueryProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	return nil
}

func (impl *libp2pRetrievalMarketNetwork) handleNewAskStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		as := &askStream{s.Conn().RemotePeer(), s, reader}
		impl.receiver.HandleAskStream(as)
	}
}

func (impl *libp2pRetrievalMarketNetwork) handleNewQueryStream(s network.Stream) {
	buffered := impl.getReaderOrReset(s)
	if buffered == nil {
		return
	}
	remotePID := s.Conn().RemotePeer()
	var qs RetrievalQueryStream
	if s.Protocol() == retrievalmarket.OldQueryProtocolID {
		qs = &oldQueryStream{remotePID, s, buffered}
	} else {
		qs = &queryStream{remotePID, s, buffered}
	}
	impl.receiver.HandleQueryStream(qs)
}

func (impl *libp2pRetrievalMarketNetwork) getReaderOrReset(s network.Stream) *bufio.Reader {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return nil
	}
	return bufio.NewReaderSize(s, 16)
}

func (impl *libp2pRetrievalMarketNetwork) ID() peer.ID {
	return impl.host.ID()
}

func (impl *libp2pRetrievalMarketNetwork) AddAddrs(p peer.ID, addrs []ma.Multiaddr) {
	impl.host.Peerstore().AddAddrs(p, addrs, 8*time.Hour)
}
