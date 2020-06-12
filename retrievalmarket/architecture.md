# Retrieval Market Architecture

# Major dependencies
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
* @hannahoward: hannahhoward/go-pubsub    

# Where things are
## types
[types.go](./types.go) is where most enumerations and interfaces for this package are defined: states and events for both 
`RetrievalProvider` and `RetrievalClient`, the `RetrievalProviderNode` and `RetrievalClientNode` interfaces.

## discovery

## impl
As you might guess, this directory contains the implementations of interfaces in [types.go](./types.go) and where the enumerations are used.
* [client.go](./impl/client.go) is where the production `RetrievalClient` is implemented. It is a handler for query and deal streams, so it implements `RetrievalReceiver` interface defined in [network/network.go](./network/network.go)
* [provider.go](./impl/provider.go) is where the production `RetrievalProvider` is implemented. It is a handler for query and deal streams, so it implements `RetrievalReceiver` interface defined in [network/network.go](./network/network.go)
* blockio
* blockunsealing
* clientstates - this directory contains state machine logic relating to the `RetrievalClient`.
    * [client_fsm.go](./impl/clientstates/client_fsm.go)  is where the state transitions are defined, and the default handlers for each new state are defined.
    * [client_fsm.go](./impl/clientstates/client_states.go) contains state handler functions.
* providerstates - contains state machine logic relating to the `RetrievalProvider`
    * [provider_fsm.go](./impl/providerstates/provider_fsm.go) is where the state transitions are defined, and the default handlers for each new state are defined.
    * [provider_fsm.go](./impl/providerstates/provider_states.go) contains state handler functions.

## network - contains basic structs and functions for sending data over data-transfer:
* [network.go](./network/network.go) - defines the interfaces that must be implemented for data-transfer.
* [deal-stream.go](./network/deal_stream.go) - implements the `RetrievalDealStream` interface, a data stream for retrieval deal traffic only
* [query-stream.go](./network/query_stream.go) - implements the `RetrievalQueryStream` interface, a data stream for retrieval query traffic only
* [libp2p_impl.go](./network/libp2p_impl.go) - libp2pRetrievalMarketNetwork implements the `RetrievalMarketNetwork` interface.

