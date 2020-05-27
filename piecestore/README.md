# The piecestore module

The piecestore module is  a simple encapsulation of two data stores, one for `PieceInfo` and
 another for `CIDInfo`.  It is currently used by the [storagemarket module](../storagemarket) for
  storing information needed for storage deals.

## Installation
```bash
go get github.com/filecoin-project/go-fil-markets/piecestore
```

## Types

* [`DealInfo`](./types.go) - information about a single deal for a given piece
* [`BlockLocation`](./types.go) - information about where a given block is relative to the
 overall
 piece
* [`PieceBlockLocation`](./types.go) - block information plus the pieceCID of the piece the
 block is inside of.
* [`CIDInfo`](./types.go) - where a given CID will live inside a piece
* [`PieceInfo`](./types.go) - metadata about a piece a provider may be storing
 based on
 its PieceCID -- so that, given a pieceCID during retrieval, the miner can determine how to unseal it if needed
* [`PieceStore`](#PieceStore) - a saved database of piece info that can be modified and
 queried

### PieceStore
`PieceStore` is a saved database of piece info that can be modified 
and queried.  The PieceStore interface is implemented in [piecestore.go](./piecestore.go).

It has two stores, one for `PieceInfo` and another for `CIDInfo`, each keyed by the `pieceCID`,
 which is a `cid.CID`.

Please see the [tests](./piecestore_test.go) for more detail about how a `PieceStore` is 
expected to behave. 

#### To create a new PieceStore
```go
func NewPieceStore(ds datastore.Batching) PieceStore
```

`PieceStore` implements the following functions:

* [`AddDealForPiece`](#AddDealForPiece)
* [`AddPieceBlockLocations`](#AddPieceBlockLocations)
* [`GetPieceInfo`](#GetPieceInfo)
* [`GetCIDInfo`](#GetCIDInfo)

#### AddDealForPiece
```go
func AddDealForPiece(pieceCID cid.Cid, dealInfo DealInfo) error
```

Store `dealInfo` in the PieceStore's piece info store, with key `pieceCID`.

#### AddPieceBlockLocations
```go
func AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]BlockLocation) error
```

Store the map of blockLocations in the PieceStore's CIDInfo store, with key `pieceCID`

#### GetPieceInfo
```go
func GetPieceInfo(pieceCID cid.Cid) (PieceInfo, error)
```

Retrieve the PieceInfo associated with `pieceCID` from the piece info store.

#### GetCIDInfo
```go
func GetCIDInfo(payloadCID cid.Cid) (CIDInfo, error)
```

Retrieve the CIDInfo associated with `pieceCID` from the CID info store.