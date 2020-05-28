# storagemarket
The storagemarket module is intended for Filecoin node implementations written in Go.
It implements functionality to allow execution of storage market deals on the
Filecoin network.
The node implementation must provide access to chain operations, and persistent 
data storage.

## Background reading

Please see the 
[Filecoin Storage Market Specification](https://filecoin-project.github.io/specs/#systems__filecoin_markets__storage_market).

## Installation
The build process for storagemarket requires Go >= v1.13.

To install:
```bash
go get github.com/filecoin-project/go-fil-markets/storagemarket
```

## Operation
The `storagemarket` package provides high level APIs to execute data storage deals between a
storage client and a storage provider (a.k.a. storage miner) on the Filecoin network.
The Filecoin node must implement the `StorageFunds`,`StorageProviderNode`, and
`StorageClientNode` interfaces in order to construct and use the module.

Deals are expected to survive a node restart; deals and related information are
 expected to be stored on disk.
 
`storagemarket` communicates its deal operations and requested data via 
                [go-data-transfer](https://github.com/filecoin-project/go-data-transfer) using 
                [go-graphsync](https://github.com/ipfs/go-graphsync).

## For Implementers
Implement the `StorageFunds`,`StorageProviderNode`, and `StorageClientNode` interfaces in 
[storagemarket/types.go](./types.go), described below:

### StorageFunds
`StorageFunds` is an interface common to both `StorageProviderNode` and `StorageClientNode`. Its
 functions are:
* [`Addfunds`](#AddFunds)
* [`EnsureFunds`](#EnsureFunds)
* [`GetBalance`](#GetBalance)
* [`VerifySignature`](#VerifySignature)
* [`WaitForMessage`](#WaitForMessage)

#### AddFunds
```go
func AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error)
```

Send `amount` to `addr by posting a message on chain. Return the message CID.

#### EnsureFunds
```go
func EnsureFunds(ctx context.Context, addr, wallet address.Address, amount abi.TokenAmount, 
            tok shared.TipSetToken) (cid.Cid, error)
```
 
Make sure `addr` has `amount` funds and if not, `wallet` should send any needed balance to
  `addr` by posting a message on chain. Returns the message CID.

#### GetBalance
```go
func GetBalance(ctx context.Context, addr address.Address, tok shared.TipSetToken) (Balance, error)
```
Retrieve the balance in `addr`

#### VerifySignature
```go
func VerifySignature(ctx context.Context, signature crypto.Signature, signer address.Address, 
                plaintext []byte, tok shared.TipSetToken) (bool, error)
```
Verify that `signature` is valid for the given `signer`, `plaintext`, and `tok`.

#### WaitForMessage
```go
func WaitForMessage(ctx context.Context, mcid cid.Cid, 
               onCompletion func(exitcode.ExitCode, []byte, error) error) error
```
Wait for message CID `mcid` to appear on chain, and call `onCompletion` when it does so.

---
### StorageProviderNode
`StorageProviderNode` is the interface for dependencies for a `StorageProvider`. It contains:

* [`StorageFunds`](#StorageFunds) interface
* [`PublishDeals`](#PublishDeals)
* [`ListProviderDeals`](#ListProviderDeals)
* [`GetMinerWorkerAddress`](#GetMinerWorkerAddress)
* [`SignBytes`](#SignBytes)
* [`OnDealSectorCommitted`](#OnDealSectorCommitted)
* [`LocatePieceForDealWithinSector`](#LocatePieceForDealWithinSector)

#### GetChainHead
```go
func GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)
```
Get the current chain head. Return its TipSetToken and its abi.ChainEpoch.

#### PublishDeals
```go
func PublishDeals(ctx context.Context, deal MinerDeal) (cid.Cid, error)
```

Post the deal to chain, returning the posted message CID.

#### ListProviderDeals
```go
func ListProviderDeals(ctx context.Context, addr address.Address, tok shared.TipSetToken,
                  ) ([]StorageDeal, error)
```

List all deals for storage provider `addr`, as of `tok`. Return a slice of `StorageDeal`.

#### OnDealComplete
```go
func OnDealComplete(ctx context.Context, deal MinerDeal, pieceSize abi.UnpaddedPieceSize, 
               pieceReader io.Reader) error
```
The function to be called when `deal` has reached the `storagemarket.StorageDealCompleted` state. 

#### GetMinerWorkerAddress
```go
func GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken,
                     ) (address.Address, error)
```

Get the miner worker address for the given miner owner, as of `tok`.

#### SignBytes
```go
func SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error)
```

Cryptographically sign bytes `b` using the private key referenced by address `signer`.

#### OnDealSectorCommitted
```go
func OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, 
                      cb DealSectorCommittedCallback) error
```

Register the function to be called once `provider` has committed sector(s) for `dealID`.

#### LocatePieceForDealWithinSector
```go
func LocatePieceForDealWithinSector(ctx context.Context, dealID abi.DealID, tok shared.TipSetToken,
                              ) (sectorID uint64, offset uint64, length uint64, err error)
```

Find the piece associated with `dealID` as of `tok` and return the sector id, plus the offset and
 length of the data within the sector.
 
---
### StorageClientNode
`StorageClientNode` implements dependencies for a StorageClient. It contains:
* [`StorageFunds`](#StorageFunds) interface
* [`GetChainHead`](#GetChainHead)
* [`ListClientDeals`](#ListClientDeals)
* [`ListStorageProviders`](#ListStorageProviders)
* [`ValidatePublishedDeal`](#ValidatePublishedDeal)
* [`SignProposal`](#SignProposal)
* [`GetDefaultWalletAddress`](#GetDefaultWalletAddress)
* [`OnDealSectorCommitted`](#OnDealSectorCommitted)
* [`ValidateAskSignature`](#ValidateAskSignature)

#### StorageFunds
`StorageClientNode` implements `StorageFunds`, described above.

#### GetChainHead
```go
func GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)
```
Get the current chain head. Return its TipSetToken and its abi.ChainEpoch.

#### ListClientDeals
```go
func ListClientDeals(ctx context.Context, addr address.Address, tok shared.TipSetToken,
                 ) ([]StorageDeal, error)
```
List all deals associated with storage client `addr`, as of `tok`. Return a slice of `StorageDeal`.

#### ListStorageProviders
```go
func ListStorageProviders(ctx context.Context, tok shared.TipSetToken) ([]*StorageProviderInfo
, error)
```

Return a slice of `StorageProviderInfo`, for all known storage providers.

#### ValidatePublishedDeal
```go
func ValidatePublishedDeal(ctx context.Context, deal ClientDeal) (abi.DealID, error)
```
Query the chain for `deal` and inspect the message parameters to make sure they match the expected  deal. Return the deal ID.

#### SignProposal
```go
func SignProposal(ctx context.Context, signer address.Address, 
                 proposal market.DealProposal) (*market.ClientDealProposal, error)
```

Cryptographically sign `proposal` using the private key of `signer` and return a
 ClientDealProposal (includes signature data).

#### GetDefaultWalletAddress
```go
func GetDefaultWalletAddress(ctx context.Context) (address.Address, error)
```

Get the default wallet address of this node, the one from which funds should be sent to the node's 
storage client or provider.

#### OnDealSectorCommitted
```go
func OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, 
                          cb DealSectorCommittedCallback) error
```

Register a callback to be called once the Deal's sector(s) are committed.

#### ValidateAskSignature
```go
func ValidateAskSignature(ctx context.Context, ask *SignedStorageAsk, tok shared.TipSetToken,
                     ) (bool, error)
```
Verify the signature in `ask`, returning true (valid) or false (invalid).