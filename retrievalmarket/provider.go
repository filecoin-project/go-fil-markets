package retrievalmarket

import "github.com/filecoin-project/specs-actors/actors/abi"

// ProviderSubscriber is a callback that is registered to listen for retrieval events on a provider
type ProviderSubscriber func(event ProviderEvent, state ProviderDealState)

// RetrievalProvider is an interface by which a provider configures their
// retrieval operations and monitors deals received and process
type RetrievalProvider interface {
	// Start begins listening for deals on the given host
	Start() error

	// Stop stops handling incoming requests
	Stop() error

	// V0

	// SetPricePerByte sets the price per byte a miner charges for retrievals
	SetPricePerByte(price abi.TokenAmount)

	// SetPaymentInterval sets the maximum number of bytes a a provider will send before
	// requesting further payment, and the rate at which that value increases
	SetPaymentInterval(paymentInterval uint64, paymentIntervalIncrease uint64)

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ProviderSubscriber) Unsubscribe

	// V1
	SetPricePerUnseal(price abi.TokenAmount)
	ListDeals() map[ProviderDealIdentifier]ProviderDealState
}
