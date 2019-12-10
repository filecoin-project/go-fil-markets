module github.com/filecoin-project/go-fil-components

go 1.13

require (
	github.com/filecoin-project/filecoin-ffi v0.0.0-20191210104338-2383ce072e95
	github.com/filecoin-project/go-fil-filestore v0.0.0-20191202230242-40c6a5a2306c
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-car v0.0.3-0.20191203022317-23b0a85fd1b1
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-merkledag v0.2.4
	github.com/ipld/go-ipld-prime v0.0.2-0.20191108012745-28a82f04c785
	github.com/stretchr/testify v1.4.0
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
