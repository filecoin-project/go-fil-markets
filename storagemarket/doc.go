/*
Package storagemarket implements the filecoin storage protocol.

An overview of the storage protocol can be found in the filecoin specification:

https://filecoin-project.github.io/specs/#systems__filecoin_markets__storage_market

Major Dependencies

Other Filecoin Repos:

https://github.com/filecoin-project/go-data-transfer - for transferring data, via go-graphsync
https://github.com/filecoin-project/go-statemachine - a finite state machine that tracks deal state
https://github.com/filecoin-project/go-storedcounter - for generating and persisting unique deal IDs
https://github.com/filecoin-project/specs-actors - the Filecoin actors

IPFS Project Repos:

https://github.com/ipfs/go-graphsync - used by go-data-transfer
https://github.com/ipfs/go-datastore - for persisting statemachine state for deals
https://github.com/ipfs/go-ipfs-blockstore - for storing and retrieving block data for deals

Other Repos:

https://github.com/libp2p/go-libp2p) the network over which retrieval deal data is exchanged.
https://github.com/hannahhoward/go-pubsub - for pub/sub notifications external to the statemachine

This top level package defines top level enumerations and interfaces. The primary implementation
lives in the `impl` directory

This package implements StorageClient & StorageProvider. The filecoin node is expected
to implement StorageClientNode & StorageProviderNode an supply them respectively as a dependency when constructing
the StorageClient & StorageProvider
*/
package storagemarket
