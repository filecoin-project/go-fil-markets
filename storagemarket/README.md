# How To Use the StorageMarket module
## Background reading

See the [Storage Market Spec](https://github.com/filecoin-project/specs/blob/master/src/systems/filecoin_markets/storage_market/_index.md) for information about how StorageMarket is expected to function.

## For Implementers
You will need to implement all of the required Client and Provider API functions in 
[storagemarket/types.go](
https://github.com/filecoin-project/go-fil-markets/blob/master/storagemarket/types.go).

### `StorageFunds`
This is an interface common to both `StorageProviderNode` and `StorageClientNode`.

#### AddFunds
`AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error)`

Send `amount` to `addr by posting a message on chain. Return the message CID.

#### EnsureFunds
`EnsureFunds(ctx context.Context, addr, wallet address.Address, amount abi.TokenAmount, tok
 shared.TipSetToken) (cid.Cid, error)`
 
Make sure `addr` has `amount` funds and if not, `wallet` should send any needed balance to
  `addr` by posting a message on chain. Returns the message CID.

#### GetBalance
`GetBalance(ctx context.Context, addr address.Address, tok shared.TipSetToken) (Balance, error)`

Retrieve the balance in `addr`l

#### VerifySignature
`VerifySignature(ctx context.Context, signature crypto.Signature, signer address.Address, plaintext []byte, tok shared.TipSetToken) (bool, error)`

Verify that `signature` is valid for the given `signer`, `plaintext`, and `tok`.

#### WaitForMessage
`WaitForMessage(ctx context.Context, mcid cid.Cid, onCompletion func(exitcode.ExitCode, []byte, error) error) error`

Wait for message CID `mcid` to appear on chain, and call `onCompletion` when it does so.

---

### `StorageProviderNode`
Node dependencies for a `StorageProvider`

#### StorageFunds
interface described above

#### GetChainHead
`GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)`

Get the current chain head.  Return the head TipSetToken and epoch for which it is the Head.

#### PublishDeals
`PublishDeals(ctx context.Context, deal MinerDeal) (cid.Cid, error)`

Post the deal to chain, returning the posted message CID.

#### ListProviderDeals
`ListProviderDeals(ctx context.Context, addr address.Address, tok shared.TipSetToken) ([]StorageDeal, error)`

List all deals for storage provider `addr`, as of `tok`. Return a slice of `StorageDeal`.

#### OnDealComplete
`OnDealComplete(ctx context.Context, deal MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceReader io.Reader) error`

The function to be called when `deal` has reached the `storagemarket.StorageDealCompleted` state. 

#### GetMinerWorkerAddress
`GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken) (address.Address, error)`

Get the miner worker address for the given miner owner, as of `tok`.

#### SignBytes
`SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error)`

Cryptographically sign bytes `b` using the private key referenced by address `signer`.

#### OnDealSectorCommitted
`OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, cb
 DealSectorCommittedCallback) error`
 
Register the function to be called once `provider` has committed sector(s) for `dealID`.

#### LocatePieceForDealWithinSector
`LocatePieceForDealWithinSector(ctx context.Context, dealID abi.DealID, tok shared.TipSetToken) (sectorID uint64, offset uint64, length uint64, err error)`

Find the piece associated with `dealID` as of `tok` and return the sector id, plus the offset and
 length of the data within the sector.

---

### StorageClientNode
Node dependencies for a StorageClient

#### StorageFunds
interface described above

#### GetChainHead
`GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)`

Get the current chain head.  Return the head TipSetToken and epoch for which it is the Head.

#### ListClientDeals
`ListClientDeals(ctx context.Context, addr address.Address, tok shared.TipSetToken) ([]StorageDeal, error)`

List all deals associated with storage client `addr`, as of `tok`. Return a slice of `StorageDeal`.

#### ListStorageProviders
`ListStorageProviders(ctx context.Context, tok shared.TipSetToken) ([]*StorageProviderInfo, error)`

Return a slice of `StorageProviderInfo`, for all known storage providers.

#### ValidatePublishedDeal
`ValidatePublishedDeal(ctx context.Context, deal ClientDeal) (abi.DealID, error)`

Query the chain for `deal` and inspect the message parameters to make sure they match the expected  deal. Return the deal ID.

#### SignProposal
`SignProposal(ctx context.Context, signer address.Address, proposal market.DealProposal) (*market.ClientDealProposal, error)`

Cryptographically sign `proposal` using the private key of `signer` and return a
 ClientDealProposal (includes signature data).

#### GetDefaultWalletAddress
`GetDefaultWalletAddress(ctx context.Context) (address.Address, error)`

Get the default wallet address of this node, the one from which funds should be sent to the node's 
storage client or provider.

#### OnDealSectorCommitted
`OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, cb DealSectorCommittedCallback) error`

Register a callback to be called once the Deal's sector(s) are committed.

#### ValidateAskSignature
`ValidateAskSignature(ctx context.Context, ask *SignedStorageAsk, tok shared.TipSetToken) (bool, error)`

Verify the signature in `ask`, returning true (valid) or false (invalid).
