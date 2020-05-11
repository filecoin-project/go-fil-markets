package network

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

// StorageAskStream is a stream for reading/writing requests &
// responses on the Storage Ask protocol
type StorageAskStream interface {
	ReadAskRequest() (AskRequest, error)
	WriteAskRequest(AskRequest) error
	ReadAskResponse() (AskResponse, error)
	WriteAskResponse(AskResponse) error
	Close() error
}

// StorageDealStream is a stream for reading and writing requests
// and responses on the storage deal protocol
type StorageDealStream interface {
	ReadDealProposal() (Proposal, error)
	WriteDealProposal(Proposal) error
	ReadDealResponse() (SignedResponse, error)
	WriteDealResponse(SignedResponse) error
	RemotePeer() peer.ID
	TagProtectedConnection(identifier string)
	UntagProtectedConnection(identifier string)
	Close() error
}

// StorageReceiver implements functions for receiving
// incoming data on storage protocols
type StorageReceiver interface {
	HandleAskStream(StorageAskStream)
	HandleDealStream(StorageDealStream)
}

// StorageMarketNetwork is a network abstraction for the storage market
type StorageMarketNetwork interface {
	NewAskStream(peer.ID) (StorageAskStream, error)
	NewDealStream(peer.ID) (StorageDealStream, error)
	SetDelegate(StorageReceiver) error
	StopHandlingRequests() error
	ID() peer.ID
}
