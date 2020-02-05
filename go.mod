module github.com/filecoin-project/go-fil-markets

go 1.13

require (
	github.com/filecoin-project/filecoin-ffi v0.0.0-20191219131535-bb699517a590
	github.com/filecoin-project/go-address v0.0.0-20191219011437-af739c490b4f
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-crypto v0.0.0-20191218222705-effae4ea9f03
	github.com/filecoin-project/go-data-transfer v0.0.0-20191219005021-4accf56bd2ce
	github.com/filecoin-project/go-padreader v0.0.0-20200130212543-892867c4edf9
	github.com/filecoin-project/go-sectorbuilder v0.0.1
	github.com/filecoin-project/go-statestore v0.1.0
	github.com/hannahhoward/cbor-gen-for v0.0.0-20191218204337-9ab7b1bcc099
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3-0.20190908200855-f22eea50656c
	github.com/ipfs/go-car v0.0.3-0.20200131220434-3f68f6ebd093
	github.com/ipfs/go-cid v0.0.4
	github.com/ipfs/go-datastore v0.1.1
	github.com/ipfs/go-graphsync v0.0.4
	github.com/ipfs/go-ipfs-blockstore v0.1.0
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-ds-help v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.4
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log/v2 v2.0.1
	github.com/ipfs/go-merkledag v0.2.4
	github.com/ipfs/go-unixfs v0.2.2-0.20190827150610-868af2e9e5cb
	github.com/ipld/go-ipld-prime v0.0.2-0.20191108012745-28a82f04c785
	github.com/ipld/go-ipld-prime-proto v0.0.0-20191113031812-e32bd156a1e5
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/libp2p/go-libp2p v0.3.0
	github.com/libp2p/go-libp2p-core v0.2.4
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/multiformats/go-multihash v0.0.10
	github.com/polydawn/refmt v0.0.0-20190809202753-05966cbd336a
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20200123233031-1cdf64d27158
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
