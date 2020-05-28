# The piecestore module

The piecestore module is a simple encapsulation of two data stores, one for `PieceInfo` and
 another for `CIDInfo`.  It is currently used by the [storagemarket module](../storagemarket) for
  storing information needed for storage deals.

## Installation
```bash
go get github.com/filecoin-project/go-fil-markets/piecestore
```

### PieceStore
`PieceStore` is primary export of this module. It is a database 
of piece info that can be modified and queried. The PieceStore 
interface is implemented in [piecestore.go](./piecestore.go).

It has two stores, one for `PieceInfo` keyed by `pieceCID`, and another for 
`CIDInfo`, keyed by `payloadCID`. These keys are of type `cid.CID`; see the 
[github.com/ipfs/go-cid](https://github.com/ipfs/go-cid) package.

Please see the [tests](./piecestore_test.go) for more detail about how a `PieceStore` is 
expected to behave. 

#### To create a new PieceStore
```go
func NewPieceStore(ds datastore.Batching) PieceStore
```

**Parameters**
* `ds datastore.Batching` is a datastore for the deal's state machine. It is
 typically the node's own datastore that implements the IPFS datastore.Batching interface.
 See
  [github.com/ipfs/go-datastore](https://github.com/ipfs/go-datastore).


`PieceStore` implements the following functions:

* [`AddDealForPiece`](./piecestore.go)
* [`AddPieceBlockLocations`](./piecestore.go)
* [`GetPieceInfo`](./piecestore.go)
* [`GetCIDInfo`](./piecestore.go)


