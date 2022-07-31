module github.com/filecoin-project/go-fil-markets

go 1.16

require (
	github.com/filecoin-project/dagstore v0.5.2
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-bitfield v0.2.4 // indirect
	github.com/filecoin-project/go-cbor-util v0.0.1
	github.com/filecoin-project/go-commp-utils v0.1.3
	github.com/filecoin-project/go-crypto v0.0.1 // indirect
	github.com/filecoin-project/go-data-transfer/v2 v2.0.0-20220731093701-aad22878e865
	github.com/filecoin-project/go-ds-versioning v0.1.1
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-fil-commp-hashhash v0.1.0
	github.com/filecoin-project/go-padreader v0.0.1
	github.com/filecoin-project/go-state-types v0.1.3
	github.com/filecoin-project/go-statemachine v1.0.3-0.20220512010507-f86db3074f9e
	github.com/filecoin-project/go-statestore v0.2.0
	github.com/filecoin-project/index-provider v0.6.1
	github.com/filecoin-project/specs-actors v0.9.13
	github.com/filecoin-project/specs-actors/v2 v2.3.6 // indirect
	github.com/filecoin-project/specs-actors/v7 v7.0.0
	github.com/filecoin-project/specs-actors/v8 v8.0.0-20220407202104-e375a8763a1c
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200817222906-ea96cece81f1
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/hashicorp/go-multierror v1.1.1
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-blockservice v0.2.1
	github.com/ipfs/go-cid v0.2.0
	github.com/ipfs/go-cidutil v0.1.0
	github.com/ipfs/go-datastore v0.5.1
	github.com/ipfs/go-filestore v1.1.0
	github.com/ipfs/go-graphsync v0.13.3-0.20220625074430-a95496cf1534
	github.com/ipfs/go-ipfs-blockstore v1.1.2
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.1.0
	github.com/ipfs/go-ipfs-exchange-offline v0.1.1
	github.com/ipfs/go-ipfs-files v0.0.9
	github.com/ipfs/go-ipld-cbor v0.0.6-0.20211211231443-5d9b9e1f6fa8
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.5.1
	github.com/ipfs/go-merkledag v0.5.1
	github.com/ipfs/go-unixfs v0.3.1
	github.com/ipld/go-car v0.3.3
	github.com/ipld/go-car/v2 v2.1.1
	github.com/ipld/go-ipld-prime v0.17.1-0.20220624062450-534ccf82237d
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jpillora/backoff v1.0.0
	github.com/libp2p/go-libp2p v0.19.4
	github.com/libp2p/go-libp2p-core v0.15.1
	github.com/multiformats/go-multiaddr v0.5.0
	github.com/multiformats/go-multibase v0.0.3
	github.com/multiformats/go-multicodec v0.5.0
	github.com/multiformats/go-multihash v0.1.0
	github.com/multiformats/go-varint v0.0.6
	github.com/petar/GoLLRB v0.0.0-20210522233825-ae3b015fd3e9
	github.com/stretchr/testify v1.7.0
	github.com/whyrusleeping/cbor v0.0.0-20171005072247-63513f603b11
	github.com/whyrusleeping/cbor-gen v0.0.0-20220302191723-37c43cae8e14
	github.com/xorcare/golden v0.6.1-0.20191112154924-b87f686d7542 // indirect
	golang.org/x/exp v0.0.0-20210715201039-d37aa40e8013
	golang.org/x/net v0.0.0-20220418201149-a630d4f3e7a2
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
