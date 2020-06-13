# Retrieval Market Architecture

## Major dependencies
* Other filecoin-project repos:
    * go-data-transfer, for transferring all information relating to an active 
        retrieval deal, via go-graphsync
    * go-statemachine, a finite state machine that tracks deal state
    * go-storedcounter, for generating and persisting unique deal IDs
    * specs-actors, the Filecoin actors
* IPFS project repos:   
    * go-graphsync, used by go-data-transfer
    * go-datastore, for persisting statemachine state for deals
    * go-ipfs-blockstore, for storing and retrieving block data for deals
* libp2p project repos:
    * go-libp2p, the network over which retrieval deal data is exchanged.
* @hannahoward: hannahhoward/go-pubsub for pub/sub notifications external to the statemachine

## Organization
### types
[types.go](./types.go) is where most enumerations and interfaces for this package are defined: states and events for both 
`RetrievalProvider` and `RetrievalClient`, the `RetrievalProviderNode` and `RetrievalClientNode` interfaces.

### discovery
contains logic for a peer registry. 

### impl
contains the implementations of interfaces in [types.go](./types.go) and where the enumerations are used.
* [client.go](./impl/client.go) is where the production `RetrievalClient` is implemented. It is a handler for query and deal streams, so it implements `RetrievalReceiver` interface defined in [network/network.go](./network/network.go)
* [provider.go](./impl/provider.go) is where the production `RetrievalProvider` is implemented. It is a handler for query and deal streams, so it implements `RetrievalReceiver` interface defined in [network/network.go](./network/network.go)
* blockio - contains the logic for a retrieval provider or client to traverse, read and verify that blocks received are in a dag in the expected traversal order.
* blockunsealing - contains the logic needed to unseal sealed blocks for retrieval.
* clientstates - this directory contains state machine logic relating to the `RetrievalClient`.
    * [client_fsm.go](./impl/clientstates/client_fsm.go)  is where the state transitions are defined, and the default handlers for each new state are defined.
    * [client_states.go](./impl/clientstates/client_states.go) contains state handler functions.
* providerstates - contains state machine logic relating to the `RetrievalProvider`
    * [provider_fsm.go](./impl/providerstates/provider_fsm.go) is where the state transitions are defined, and the default handlers for each new state are defined.
    * [provider_states.go](./impl/providerstates/provider_states.go) contains state handler functions.

### network
contains basic structs and functions for sending data over data-transfer:
* [network.go](./network/network.go) - defines the interfaces that must be implemented for data-transfer.
* [deal-stream.go](./network/deal_stream.go) - implements the `RetrievalDealStream` interface, a data stream for retrieval deal traffic only
* [query-stream.go](./network/query_stream.go) - implements the `RetrievalQueryStream` interface, a data stream for retrieval query traffic only
* [libp2p_impl.go](./network/libp2p_impl.go) - implements the `RetrievalMarketNetwork` interface.

## Setup for handling data over data-transfer
On the client side, the Client just has to be initialized and then it's ready to start querying or proposing retrieval deals.
Query and Deal streams are created in each Query and Retrieve function.

On the provider side, to allow receiving of queries or deal proposals, `Provider.Start` function must be called, 
which simply calls its `network.SetDelegate` function, passing itself as the `RetrievalReceiver`. 
It constructs a new statemachine, passing itself as the statemachine environment and its `notifySubscribers` 
function as the notifier.


## RetrievalQuery Flow
* A retrieval query would be used to determine who has a desired payload CID. This utilizes 
the `FindProviders` function of the `PeerResolver` interface implementation.

* Next the retrieval Client's `Query` function is called. This creates a new `RetrievalQueryStream` for the chosen peer ID, 
and calls WriteQuery on it, which constructs a data-transfer message and writes it to the Query stream.

* A Provider handling a retrieval `Query` will need to get the node's chain head, its miner worker address, and look
in its piece store for the requested piece info. It uses this to construct a response which includes whether the
piece is there, and if so the retrieval deal terms in its `retrievalmarket.QueryResponse` struct.  It then writes this
response to the `Query` stream.

The connection is kept open only as long as the query-response exchange.

## RetrievalDeal Flow
Deal flow involves multiple exchanges.

* Similarly to `Query`, client Retrieve creates a new `RetrievalDealStream`.  At the time of this writing, this connection is 
kept open through the entire deal until completion or failure.  There are plans to make this pauseable as well as surviving
a restart.

* Retrieval flow not only uses the [network](./network/network.go) interfaces, but also storedcounter, piecestore, statemachine, and blockstore. 
It calls the node API for chain operations such as `GetHead` but also to create payment channels.

* When the Client determines a peer in possession of desired data for acceptable deal terms, `Retrieve` is called for the 
data and the peer, sending
what should be acceptable deal terms received from in the peer's `QueryResponse` and launches the deal flow:

1. The client creates a deal ID using the next value from its storedcounter.
1. Constructs a `DealProposal` with deal terms
1. Tells its statemachine to begin tracking this deal state by dealID.
1. constructs a `blockio.SelectorVerifier` and adds it to its dealID-keyed map of block verifiers.
1. triggers a `ClientEventOpen` event on its statemachine.

From then on, the statemachine controls the deal flow in the client. Other components may listen for events in this flow by calling
`SubscribeToEvents` on the Client. The Client handles consuming blocks it receives from the provider, via `ConsumeBlocks` function.

### How the statemachine works for a retrieval deal, client side

In addition to defining the state transition behavior, There is a list of entry funcs, `ClientStateEntryFuncs` in [impl/clientstates/client_fsm.go](./impl/clientstates/client_fsm.go) which map a 
state to a function that is invoked on an event trigger. Not all events map to an entry function.

The entry functions are defined in [impl/clientstates/client_states.go](./impl/clientstates/client_states.go)

Under normal operation, for an accepted deal with no errors, restarts or pauses, assuming enough data 
is requested to warrant incremental vouchers, the **`event`** --> `state` transitions should go as follows: 

1. **`ClientEventOpen`** --> `DealStatusNew`
1. **`ClientEventDealAccepted`** --> `DealStatusAccepted`
1. **`ClientEventPaymentChannelCreateInitiated`** --> `DealStatusPaymentChannelCreating` 
   OR **`ClientEventPaymentChannelAddingFunds`** --> `DealStatusPaymentChannelAddingFunds` (for an existing payment channel)
1. **`ClientEventPaymentChannelReady`** --> `DealStatusPaymentChannelReady`
1. ( **`ClientEventLastPaymentRequested`** --> `DealStatusFundsNeeded`
1.   **`ClientEventPaymentSent`** --> `DealStatusOngoing` ) 
     this and the previous event-transition may cycle multiple times.
1. **`ClientEventLastPaymentRequested`** --> `DealStatusFundsNeededLastPayment`
1. **`ClientEventPaymentSent`** -> `DealStatusFinalizing`
1. **`ClientEventComplete`** --> `DealStatusCompleted`