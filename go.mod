module github.com/filecoin-project/go-fil-markets

go 1.13

require (
	github.com/filecoin-project/dagstore v0.5.0
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-commp-utils v0.1.1-0.20210427191551-70bf140d31c7
	github.com/filecoin-project/go-data-transfer v1.11.6
	github.com/filecoin-project/go-ds-versioning v0.1.0
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-fil-commp-hashhash v0.1.0
	github.com/filecoin-project/go-padreader v0.0.0-20210723183308-812a16dc01b1
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/go-statemachine v0.0.0-20200925024713-05bd7c71fbfe
	github.com/filecoin-project/go-statestore v0.1.1
	github.com/filecoin-project/index-provider v0.0.0-20211116211010-ae6b83454d89
	github.com/filecoin-project/specs-actors v0.9.13
	github.com/filecoin-project/specs-actors/v2 v2.3.5-0.20210114162132-5b58b773f4fb
	github.com/filecoin-project/storetheindex v0.0.0-20211019180831-2704585e5f99
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200817222906-ea96cece81f1
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-blockservice v0.1.7
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-cidutil v0.0.2
	github.com/ipfs/go-datastore v0.4.6
	github.com/ipfs/go-filestore v1.0.0
	github.com/ipfs/go-graphsync v0.10.4
	github.com/ipfs/go-ipfs-blockstore v1.0.4
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/ipfs/go-merkledag v0.4.0
	github.com/ipfs/go-unixfs v0.2.6
	github.com/ipld/go-car v0.3.2
	github.com/ipld/go-car/v2 v2.1.0
	github.com/ipld/go-ipld-prime v0.14.0
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jpillora/backoff v1.0.0
	github.com/libp2p/go-libp2p v0.15.0
	github.com/libp2p/go-libp2p-core v0.9.0
	github.com/multiformats/go-multiaddr v0.4.1
	github.com/multiformats/go-multibase v0.0.3
	github.com/multiformats/go-multihash v0.1.0
	github.com/stretchr/testify v1.7.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20210713220151-be142a5ae1a8
	golang.org/x/exp v0.0.0-20210715201039-d37aa40e8013
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
