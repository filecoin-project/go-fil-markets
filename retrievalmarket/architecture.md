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
    * [client_fsm.go](./impl/clientstates/client_states.go) contains state handler functions.
* providerstates - contains state machine logic relating to the `RetrievalProvider`
    * [provider_fsm.go](./impl/providerstates/provider_fsm.go) is where the state transitions are defined, and the default handlers for each new state are defined.
    * [provider_fsm.go](./impl/providerstates/provider_states.go) contains state handler functions.

### network
contains basic structs and functions for sending data over data-transfer:
* [network.go](./network/network.go) - defines the interfaces that must be implemented for data-transfer.
* [deal-stream.go](./network/deal_stream.go) - implements the `RetrievalDealStream` interface, a data stream for retrieval deal traffic only
* [query-stream.go](./network/query_stream.go) - implements the `RetrievalQueryStream` interface, a data stream for retrieval query traffic only
* [libp2p_impl.go](./network/libp2p_impl.go) - implements the `RetrievalMarketNetwork` interface.

## Setup for handling data over data-transfer
On the client side, the Client just has to be initialized and then it's ready to start querying or proposing retrieval deals.
Query and Deal streams are created in each Query and Retrieve function.

On the provider side, to allow receiving of queries or deal proposals, Provider.Start function must be called, 
which simply calls its network.SetDelegate function, passing itself as the RetrievalReceiver.

## RetrievalQuery Flow
A retrieval query would be used to determine who has a desired payload CID. FindProviders would be used and this utilizes 
the FindProviders function of the PeerResolver interface implementation.

Next the retrieval Client's Query function would be called and this creates a new RetrievalQueryStream for the chosen peer ID, 
and calls WriteQuery on it, which constructs a data-transfer message and writes it to the Query stream.

A Provider handling a retrieval Query will need to get the node's chain head, its miner worker address, and look
in its piece store for the requested piece info. It uses this to construct a response which includes whether the
piece is there, and if so the retrieval deal terms in its retrievalmarket.QueryResponse struct.  It then writes this
response to the Query stream.

## RetrievalDeal Flow
The deal flow involves multiple exchanges. 