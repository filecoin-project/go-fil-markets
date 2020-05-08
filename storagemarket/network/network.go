package network

import (
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/libp2p/go-libp2p-core/peer"
)

// StorageAskStream is a stream for reading/writing requests &
// responses on the Storage Ask protocol
type StorageAskStream interface {
	ReadAskRequest() (storagemarket.AskRequest, error)
	WriteAskRequest(storagemarket.AskRequest) error
	ReadAskResponse() (storagemarket.AskResponse, error)
	WriteAskResponse(storagemarket.AskResponse) error
	Close() error
}

// StorageDealStream is a stream for reading and writing requests
// and responses on the storage deal protocol
type StorageDealStream interface {
	ReadDealProposal() (storagemarket.ProposalRequest, error)
	WriteDealProposal(storagemarket.ProposalRequest) error
	ReadDealResponse() (storagemarket.SignedResponse, error)
	WriteDealResponse(storagemarket.SignedResponse) error
	RemotePeer() peer.ID
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
