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

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

const defaultMaxStreamOpenAttempts = 5
const defaultMinAttemptDuration = 1 * time.Second
const defaultMaxAttemptDuration = 5 * time.Minute

var log = logging.Logger("retrieval_network")
var _ RetrievalMarketNetwork = new(libp2pRetrievalMarketNetwork)

// Option is an option for configuring the libp2p storage market network
type Option func(*libp2pRetrievalMarketNetwork)

// RetryParameters changes the default parameters around connection reopening
func RetryParameters(minDuration time.Duration, maxDuration time.Duration, attempts float64) Option {
	return func(impl *libp2pRetrievalMarketNetwork) {
		impl.maxStreamOpenAttempts = attempts
		impl.minAttemptDuration = minDuration
		impl.maxAttemptDuration = maxDuration
	}
}

// SupportedProtocols sets what protocols this network instances listens on
func SupportedProtocols(supportedProtocols []protocol.ID) Option {
	return func(impl *libp2pRetrievalMarketNetwork) {
		impl.supportedProtocols = supportedProtocols
	}
}

// NewFromLibp2pHost constructs a new instance of the RetrievalMarketNetwork from a
// libp2p host
func NewFromLibp2pHost(h host.Host, options ...Option) RetrievalMarketNetwork {
	impl := &libp2pRetrievalMarketNetwork{
		host:                  h,
		maxStreamOpenAttempts: defaultMaxStreamOpenAttempts,
		minAttemptDuration:    defaultMinAttemptDuration,
		maxAttemptDuration:    defaultMaxAttemptDuration,
		supportedProtocols: []protocol.ID{
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
	host host.Host
	// inbound messages from the network are forwarded to the receiver
	receiver              RetrievalReceiver
	maxStreamOpenAttempts float64
	minAttemptDuration    time.Duration
	maxAttemptDuration    time.Duration
	supportedProtocols    []protocol.ID
}

//  NewQueryStream creates a new RetrievalQueryStream using the provided peer.ID
func (impl *libp2pRetrievalMarketNetwork) NewQueryStream(id peer.ID) (RetrievalQueryStream, error) {
	s, err := impl.openStream(context.Background(), id, impl.supportedProtocols)
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

func (impl *libp2pRetrievalMarketNetwork) openStream(ctx context.Context, id peer.ID, protocols []protocol.ID) (network.Stream, error) {
	b := &backoff.Backoff{
		Min:    impl.minAttemptDuration,
		Max:    impl.maxAttemptDuration,
		Factor: impl.maxStreamOpenAttempts,
		Jitter: true,
	}

	for {
		s, err := impl.host.NewStream(ctx, id, protocols...)
		if err == nil {
			return s, err
		}

		nAttempts := b.Attempt()
		if nAttempts == impl.maxStreamOpenAttempts {
			return nil, xerrors.Errorf("exhausted %d attempts but failed to open stream, err: %w", int(impl.maxStreamOpenAttempts), err)
		}
		ebt := time.NewTimer(b.Duration())
		select {
		case <-ctx.Done():
			ebt.Stop()
			return nil, xerrors.Errorf("backoff canceled by context")
		case <-ebt.C:
		}
	}
}

// SetDelegate sets a RetrievalReceiver to handle stream data
func (impl *libp2pRetrievalMarketNetwork) SetDelegate(r RetrievalReceiver) error {
	impl.receiver = r
	for _, proto := range impl.supportedProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewQueryStream)
	}
	return nil
}

// StopHandlingRequests unsets the RetrievalReceiver and would perform any other necessary
// shutdown logic.
func (impl *libp2pRetrievalMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	for _, proto := range impl.supportedProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	return nil
}

func (impl *libp2pRetrievalMarketNetwork) handleNewQueryStream(s network.Stream) {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return
	}
	remotePID := s.Conn().RemotePeer()
	buffered := bufio.NewReaderSize(s, 16)
	var qs RetrievalQueryStream
	if s.Protocol() == retrievalmarket.OldQueryProtocolID {
		qs = &oldQueryStream{remotePID, s, buffered}
	} else {
		qs = &queryStream{remotePID, s, buffered}
	}
	impl.receiver.HandleQueryStream(qs)
}

func (impl *libp2pRetrievalMarketNetwork) ID() peer.ID {
	return impl.host.ID()
}

func (impl *libp2pRetrievalMarketNetwork) AddAddrs(p peer.ID, addrs []ma.Multiaddr) {
	impl.host.Peerstore().AddAddrs(p, addrs, 8*time.Hour)
}
