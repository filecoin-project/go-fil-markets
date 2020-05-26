# How to use the RetrievalMarket module

## Background reading
Please see the 
[Filecoin Retrieval Market Specification](https://filecoin-project.github.io/specs/#systems__filecoin_markets__retrieval_market)

## For Implementers
You will need to implement all of the required Client and Provider API functions in 
[retrievalmarket/types.go](./types.go), described below:

### PeerResolver
PeerResolver is an interface for looking up providers that may have a piece.

#### GetPeers
```go
GetPeers(payloadCID cid.Cid) ([]RetrievalPeer, error)
```
Return a slice of RetrievalPeers that store data referenced by `payloadCID`

---
### RetrievalClientNode

`RetrievalClientNode` contains the node dependencies for a RetrievalClient.

* [`AllocateLane`](#AllocateLane)
* [`GetChainHead`](#GetChainHead)
* [`GetOrCreatePaymentChannel`](#GetOrCreatePaymentChannel)
* [`CreatePaymentVoucher`](#CreatePaymentVoucher)
* [`WaitForPaymentChannelAddFunds`](#WaitForPaymentChannelAddFunds)
* [`WaitForPaymentChannelCreation`](#WaitForPaymentChannelCreation)

#### AllocateLane
```go
AllocateLane(paymentChannel address.Address) (uint64, error)
```

Create a lane within `paymentChannel` so that calls to CreatePaymentVoucher will 
automatically make vouchers only for the difference in total

#### CreatePaymentVoucher
```go
CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount, 
            		lane uint64, tok shared.TipSetToken) (*paych.SignedVoucher, error)
```
Create a new payment voucher for `paymentChannel` with `amount`, for lane `lane`, given chain
state at `tok`

#### GetChainHead
```go
GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)
```
Get the current chain head. Return the head TipSetToken and abi.ChainEpoch for 
which it is the Head.

#### GetOrCreatePaymentChannel
```go
GetOrCreatePaymentChannel(ctx context.Context, clientAddress, minerAddress address.Address, 
                          amount abi.TokenAmount, tok shared.TipSetToken
                         ) (address.Address, cid.Cid, error)
```
If there is a current payment channel for deals between `clientAddress` and `minerAddress`, 
add `amount` to the channel, then return the payment channel address and `cid.Undef`.

If there isn't, construct a new payment channel actor with `amount` funds by posting 
the corresponding message on chain, then return `address.Undef` and the posted message `cid.Cid`.

#### WaitForPaymentChannelAddFunds
```go
WaitForPaymentChannelAddFunds(messageCID cid.Cid) error
```
Wait for message with CID `messageCID` on chain that funds have been sent to a payment channel.

#### WaitForPaymentChannelCreation
```go
WaitForPaymentChannelCreation(messageCID cid.Cid) (address.Address, error)
```
Wait for a message on chain with CID `messageCID` that a payment channel has been created.

---
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
GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken,
                     ) (address.Address, error)
```
Get the miner worker address for the given miner owner, as of `tok`.

#### UnsealSector
```go
UnsealSector(ctx context.Context, sectorID uint64, offset uint64, length uint64,
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

---
