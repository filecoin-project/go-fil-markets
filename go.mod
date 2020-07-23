module github.com/filecoin-project/go-fil-markets

go 1.13

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200218010043-eb9bb40ed5be
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-data-transfer v0.5.0
	github.com/filecoin-project/go-padreader v0.0.0-20200210211231-548257017ca6
	github.com/filecoin-project/go-statemachine v0.0.0-20200714194326-a77c3ae20989
	github.com/filecoin-project/go-statestore v0.1.0
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/sector-storage v0.0.0-20200615154852-728a47ab99d6
	github.com/filecoin-project/specs-actors v0.8.1-0.20200720115956-cd051eabf328
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200723175505-5892b522820a
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.6
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-graphsync v0.0.6-0.20200715204712-ef06b3d32e83
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-cbor v0.0.4
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.0.5
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipld/go-car v0.1.1-0.20200429200904-c222d793c339
	github.com/ipld/go-ipld-prime v0.0.2-0.20200428162820-8b59dc292b8e
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/libp2p/go-libp2p v0.10.0
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/stretchr/testify v1.6.1
	github.com/whyrusleeping/cbor-gen v0.0.0-20200723182808-cb5de1c427f5
	golang.org/x/exp v0.0.0-20190121172915-509febef88a4
	golang.org/x/net v0.0.0-20190923162816-aa69164e4478
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
