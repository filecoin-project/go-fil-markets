module github.com/filecoin-project/go-fil-markets

go 1.13

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/filecoin-project/dagstore v0.0.0-20210624112130-607a5a2a5bd4
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-amt-ipld/v2 v2.1.1-0.20201006184820-924ee87a1349 // indirect
	github.com/filecoin-project/go-bitfield v0.2.4 // indirect
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-commp-utils v0.1.0
	github.com/filecoin-project/go-data-transfer v1.7.0
	github.com/filecoin-project/go-ds-versioning v0.1.0
	github.com/filecoin-project/go-multistore v0.0.3
	github.com/filecoin-project/go-padreader v0.0.0-20200903213702-ed5fae088b20
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/go-statemachine v0.0.0-20200925024713-05bd7c71fbfe
	github.com/filecoin-project/go-statestore v0.1.1
	github.com/filecoin-project/specs-actors v0.9.13
	github.com/filecoin-project/specs-actors/v2 v2.3.5-0.20210114162132-5b58b773f4fb
	github.com/google/gopacket v1.1.18 // indirect
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200817222906-ea96cece81f1
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/ipfs/go-bitswap v0.3.2 // indirect
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-blockservice v0.1.4
	github.com/ipfs/go-cid v0.0.8-0.20210702173502-41f2377d9672
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-graphsync v0.6.4
	github.com/ipfs/go-ipfs-blockstore v1.0.3
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.1.3
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-unixfs v0.2.6
	github.com/ipld/go-car v0.1.1-0.20201119040415-11b6074b6d4d
	github.com/ipld/go-car/v2 v2.0.0-20210702174845-f0f4cd8bd521
	github.com/ipld/go-ipld-prime v0.5.1-0.20201021195245-109253e8a018
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jpillora/backoff v1.0.0
	github.com/libp2p/go-libp2p v0.12.0
	github.com/libp2p/go-libp2p-core v0.7.0
	github.com/libp2p/go-libp2p-noise v0.1.2 // indirect
	github.com/libp2p/go-libp2p-record v0.1.3 // indirect
	github.com/libp2p/go-libp2p-yamux v0.4.1 // indirect
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/multiformats/go-multibase v0.0.3
	github.com/multiformats/go-multihash v0.0.15
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/onsi/ginkgo v1.14.0 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.2.0 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20210219115102-f37d292932f2
	github.com/xorcare/golden v0.6.1-0.20191112154924-b87f686d7542 // indirect
	go.opencensus.io v0.22.5 // indirect
	golang.org/x/exp v0.0.0-20210615023648-acb5c1269671
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
