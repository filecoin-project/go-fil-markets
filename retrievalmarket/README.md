# How to use the RetrievalMarket module

## Background reading
Please see the [Filecoin Retrieval Market Specification](https://filecoin-project.github.io/specs
/#systems__filecoin_markets__retrieval_market)

## For Implementers
You will need to implement all of the required Client and Provider API functions in 
[retrievalmarket/types.go](./types.go), described below:

### RetrievalProviderNode
`RetrievalProviderNode` contains the node dependencies for a RetrievalProvider.

* [`GetChainHead`](#GetChainHead)
* [`GetMinerWorkerAddress`](#GetMinerWorkerAddress)
* [`UnsealSector`](#UnsealSector)
* [`SavePaymentVoucher`](#SavePaymentVoucher)

#### GetChainHead
```go
GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)
```
Get the current chain head. Return the head TipSetToken and abi.ChainEpoch for 
which it is the Head.

#### GetMinerWorkerAddress
```go
GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken
) (address.Address, error)
```
Get the miner worker address for the given miner owner, as of `tok`.

#### UnsealSector
```go
UnsealSector(ctx context.Context, sectorID uint64, offset uint64, length uint64
             ) (io.ReadCloser, error)
```
Unseal `length` data contained in `sectorID`, starting at `offset`.  Return an `io.ReadCloser
` for accessing the data.

#### SavePaymentVoucher
```go
SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, 
                   voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, 
                   tok shared.TipSetToken) (abi.TokenAmount, error)
```

Save the provided `paych.SignedVoucher` for `paymentChannel`. The RetrievalProviderNode
implementation is expected to validate the SignedVoucher using the provided `proof`, `
expectedAmount`, based on  the chain state referenced by `tok`.  The value of the
voucher should be equal or greater than the largest previous voucher by 
 `expectedAmount`. It returns the actual difference.
