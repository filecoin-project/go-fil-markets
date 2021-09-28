package network

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

// These are the required interfaces that must be implemented to send and receive data
// for retrieval queries and deals.

// RetrievalAskStream is a stream for reading/writing requests &
// responses on the Retrieval Ask protocol
type RetrievalAskStream interface {
	ReadAskRequest() (AskRequest, error)
	WriteAskRequest(AskRequest) error
	ReadAskResponse() (AskResponse, []byte, error)
	WriteAskResponse(AskResponse) error
	Close() error
}

// RetrievalQueryStream is the API needed to send and receive retrieval query
// data over data-transfer network.
type RetrievalQueryStream interface {
	ReadQuery() (retrievalmarket.Query, error)
	WriteQuery(retrievalmarket.Query) error
	ReadQueryResponse() (retrievalmarket.QueryResponse, error)
	WriteQueryResponse(retrievalmarket.QueryResponse) error
	Close() error
	RemotePeer() peer.ID
}

// RetrievalReceiver is the API for handling data coming in on
// both query and deal streams
type RetrievalReceiver interface {
	// HandleAskStream reads queries and writes responses to the ask stream
	HandleAskStream(RetrievalAskStream)
	// HandleQueryStream sends and receives data-transfer data via the
	// RetrievalQueryStream provided
	HandleQueryStream(RetrievalQueryStream)
}

// RetrievalMarketNetwork is the API for creating query and deal streams and
// delegating responders to those streams.
type RetrievalMarketNetwork interface {
	// Open a stream to the provider to request the provider's ask
	NewAskStream(ctx context.Context, id peer.ID) (RetrievalAskStream, error)
	//  NewQueryStream creates a new RetrievalQueryStream implementer using the provided peer.ID
	NewQueryStream(peer.ID) (RetrievalQueryStream, error)

	// SetDelegate sets a RetrievalReceiver implementer to handle stream data
	SetDelegate(RetrievalReceiver) error

	// StopHandlingRequests unsets the RetrievalReceiver and would perform any other necessary
	// shutdown logic.
	StopHandlingRequests() error

	// ID returns the peer id of the host for this network
	ID() peer.ID

	// AddAddrs adds the given multi-addrs to the peerstore for the passed peer ID
	AddAddrs(peer.ID, []ma.Multiaddr)
}
