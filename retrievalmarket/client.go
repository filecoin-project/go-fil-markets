package retrievalmarket

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

// ClientSubscriber is a callback that is registered to listen for retrieval events
type ClientSubscriber func(event ClientEvent, state ClientDealState)

// RetrievalClient is a client interface for making retrieval deals
type RetrievalClient interface {
	// V0

	// Find Providers finds retrieval providers who may be storing a given piece
	FindProviders(payloadCID cid.Cid) []RetrievalPeer

	// Query asks a provider for information about a piece it is storing
	Query(
		ctx context.Context,
		p RetrievalPeer,
		payloadCID cid.Cid,
		params QueryParams,
	) (QueryResponse, error)

	// Retrieve retrieves all or part of a piece with the given retrieval parameters
	Retrieve(
		ctx context.Context,
		payloadCID cid.Cid,
		params Params,
		totalFunds abi.TokenAmount,
		p RetrievalPeer,
		clientWallet address.Address,
		minerWallet address.Address,
		storeID *multistore.StoreID,
	) (DealID, error)

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ClientSubscriber) Unsubscribe

	// V1

	// TryRestartInsufficientFunds attempts to restart any deals stuck in the insufficient funds state
	// after funds are added to a given payment channel
	TryRestartInsufficientFunds(paymentChannel address.Address) error

	// CancelDeal attempts to cancel an inprogress deal
	CancelDeal(id DealID) error

	// GetDeal returns a given deal by deal ID, if it exists
	GetDeal(dealID DealID) (ClientDealState, error)

	// ListDeals returns all deals
	ListDeals() (map[DealID]ClientDealState, error)
}
