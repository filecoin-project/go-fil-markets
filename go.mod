module github.com/filecoin-project/go-fil-markets

go 1.13

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200716204036-cddc56607e1d
	github.com/filecoin-project/go-address v0.0.3
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-data-transfer v0.6.8-0.20201013092017-eee8d1cde5d5
	github.com/filecoin-project/go-ds-versioning v0.1.0
	github.com/filecoin-project/go-multistore v0.0.3
	github.com/filecoin-project/go-padreader v0.0.0-20200903213702-ed5fae088b20
	github.com/filecoin-project/go-state-types v0.0.0-20200928172055-2df22083d8ab
	github.com/filecoin-project/go-statemachine v0.0.0-20200925024713-05bd7c71fbfe
	github.com/filecoin-project/go-statestore v0.1.0
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/specs-actors v0.9.12
	github.com/filecoin-project/specs-actors/v2 v2.0.1
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200817222906-ea96cece81f1
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.4-0.20200624145336-a978cec6e834
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-graphsync v0.2.1-0.20201013053840-5d8ea8076a2c
	github.com/ipfs/go-ipfs-blockstore v1.0.1
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200204214505-252690b78669 // indirect
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.0.5
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipld/go-car v0.1.1-0.20200923150018-8cdef32e2da4
	github.com/ipld/go-ipld-prime v0.5.1-0.20200828233916-988837377a7f
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/jpillora/backoff v1.0.0
	github.com/libp2p/go-libp2p v0.10.0
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multibase v0.0.3
	github.com/prometheus/common v0.0.0-20180801064454-c7de2306084e
	github.com/stretchr/testify v1.6.1
	github.com/whyrusleeping/cbor-gen v0.0.0-20200826160007-0b9f6c5fb163
	golang.org/x/exp v0.0.0-20200207192155-f17229e696bd
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
