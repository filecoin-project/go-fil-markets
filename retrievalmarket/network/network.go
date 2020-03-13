package network

import (
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type RetrievalQueryStream interface {
	ReadQuery() (retrievalmarket.Query, error)
	WriteQuery(retrievalmarket.Query) error
	ReadQueryResponse() (retrievalmarket.QueryResponse, error)
	WriteQueryResponse(retrievalmarket.QueryResponse) error
	Close() error
}

type RetrievalDealStream interface {
	ReadDealProposal() (retrievalmarket.DealProposal, error)
	WriteDealProposal(retrievalmarket.DealProposal) error
	ReadDealResponse() (retrievalmarket.DealResponse, error)
	WriteDealResponse(retrievalmarket.DealResponse) error
	ReadDealPayment() (retrievalmarket.DealPayment, error)
	WriteDealPayment(retrievalmarket.DealPayment) error
	Receiver() peer.ID
	Close() error
}

type RetrievalReceiver interface {
	HandleQueryStream(RetrievalQueryStream)
	HandleDealStream(RetrievalDealStream)
}

type RetrievalMarketNetwork interface {
	NewQueryStream(peer.ID) (RetrievalQueryStream, error)
	NewDealStream(peer.ID) (RetrievalDealStream, error)
	SetDelegate(RetrievalReceiver) error
	StopHandlingRequests() error
}
