package testnodes

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// Below fake node implementations

// TestStateKey is just a stubbed state key that returns a preset height
type TestStateKey struct{ Epoch abi.ChainEpoch }

// Height returns the value specified by Epoch
func (k *TestStateKey) Height() abi.ChainEpoch {
	return k.Epoch
}

// StorageMarketState represents a state for the storage market that can be inspected
// - methods on the provider nodes will affect this state
type StorageMarketState struct {
	Epoch        abi.ChainEpoch
	DealId       abi.DealID
	Balances     map[address.Address]abi.TokenAmount
	StorageDeals map[address.Address][]storagemarket.StorageDeal
	Providers    []*storagemarket.StorageProviderInfo
}

// NewStorageMarketState returns a new empty state for the storage market
func NewStorageMarketState() *StorageMarketState {
	return &StorageMarketState{
		Epoch:        0,
		DealId:       0,
		Balances:     map[address.Address]abi.TokenAmount{},
		StorageDeals: map[address.Address][]storagemarket.StorageDeal{},
		Providers:    nil,
	}
}

// AddFunds adds funds for a given address in the storage market
func (sma *StorageMarketState) AddFunds(addr address.Address, amount abi.TokenAmount) {
	if existing, ok := sma.Balances[addr]; ok {
		sma.Balances[addr] = big.Add(existing, amount)
	} else {
		sma.Balances[addr] = amount
	}
}

// Balance returns the balance of a given address in the market
func (sma *StorageMarketState) Balance(addr address.Address) storagemarket.Balance {
	if existing, ok := sma.Balances[addr]; ok {
		return storagemarket.Balance{Locked: big.NewInt(0), Available: existing}
	}
	return storagemarket.Balance{Locked: big.NewInt(0), Available: big.NewInt(0)}
}

// Deals returns all deals in the current state
func (sma *StorageMarketState) Deals(addr address.Address) []storagemarket.StorageDeal {
	if existing, ok := sma.StorageDeals[addr]; ok {
		return existing
	}
	return nil
}

// StateKey returns a state key with the storage market states set Epoch
func (sma *StorageMarketState) StateKey() storagemarket.StateKey {
	return &TestStateKey{sma.Epoch}
}

// AddDeal adds a deal to the current state of the storage market
func (sma *StorageMarketState) AddDeal(deal storagemarket.StorageDeal) storagemarket.StateKey {
	for _, addr := range []address.Address{deal.Client, deal.Provider} {
		if existing, ok := sma.StorageDeals[addr]; ok {
			sma.StorageDeals[addr] = append(existing, deal)
		} else {
			sma.StorageDeals[addr] = []storagemarket.StorageDeal{deal}
		}
	}
	return sma.StateKey()
}

// FakeCommonNode has the common methods for the storage & client node adapters
type FakeCommonNode struct {
	SMState                *StorageMarketState
	EnsureFundsError       error
	VerifySignatureFails   bool
	GetBalanceError        error
	MostRecentStateIDError error
}

// MostRecentStateId returns the state id in the storage market state
func (n *FakeCommonNode) MostRecentStateId(ctx context.Context) (storagemarket.StateKey, error) {
	if n.MostRecentStateIDError == nil {
		return n.SMState.StateKey(), nil
	}
	return &TestStateKey{}, n.MostRecentStateIDError
}

// AddFunds adds funds to the given actor in the storage market state
func (n *FakeCommonNode) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	n.SMState.AddFunds(addr, amount)
	return nil
}

// EnsureFunds adds funds to the given actor in the storage market state to insure it has at least the given amount
func (n *FakeCommonNode) EnsureFunds(ctx context.Context, addr, wallet address.Address, amount abi.TokenAmount) error {
	if n.EnsureFundsError == nil {
		balance := n.SMState.Balance(addr)
		if balance.Available.LessThan(amount) {
			n.SMState.AddFunds(addr, big.Sub(amount, balance.Available))
		}
	}
	return n.EnsureFundsError
}

// GetBalance returns the funds in the storage market state
func (n *FakeCommonNode) GetBalance(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	if n.GetBalanceError == nil {
		return n.SMState.Balance(addr), nil
	}
	return storagemarket.Balance{}, n.GetBalanceError
}

// VerifySignature just always returns true, for now
func (n *FakeCommonNode) VerifySignature(signature crypto.Signature, addr address.Address, data []byte) bool {
	return !n.VerifySignatureFails
}

// FakeClientNode implements functions specific to the StorageClientNode
type FakeClientNode struct {
	FakeCommonNode
	ClientAddr              address.Address
	ValidationError         error
	ValidatePublishedDealID abi.DealID
	ValidatePublishedError  error
	DealCommittedSyncError  error
	DealCommittedAsyncError error
}

// ListClientDeals just returns the deals in the storage market state
func (n *FakeClientNode) ListClientDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	return n.SMState.Deals(addr), nil
}

// ListStorageProviders lists the providers in the storage market state
func (n *FakeClientNode) ListStorageProviders(ctx context.Context) ([]*storagemarket.StorageProviderInfo, error) {
	return n.SMState.Providers, nil
}

// ValidatePublishedDeal always succeeds
func (n *FakeClientNode) ValidatePublishedDeal(ctx context.Context, deal storagemarket.ClientDeal) (abi.DealID, error) {
	return n.ValidatePublishedDealID, n.ValidatePublishedError
}

// SignProposal signs a deal with a dummy signature
func (n *FakeClientNode) SignProposal(ctx context.Context, signer address.Address, proposal market.DealProposal) (*market.ClientDealProposal, error) {
	return &market.ClientDealProposal{
		Proposal:        proposal,
		ClientSignature: *shared_testutil.MakeTestSignature(),
	}, nil
}

// GetDefaultWalletAddress returns the address specified by ClientAddr
func (n *FakeClientNode) GetDefaultWalletAddress(ctx context.Context) (address.Address, error) {
	return n.ClientAddr, nil
}

// OnDealSectorCommitted returns immediately, with success
func (n *FakeClientNode) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, cb storagemarket.DealSectorCommittedCallback) error {
	if n.DealCommittedSyncError == nil {
		cb(n.DealCommittedAsyncError)
	}
	return n.DealCommittedSyncError
}

// ValidateAskSignature returns the stubbed validation error
func (n *FakeClientNode) ValidateAskSignature(ask *storagemarket.SignedStorageAsk) error {
	return n.ValidationError
}

var _ storagemarket.StorageClientNode = (*FakeClientNode)(nil)

// FakeProviderNode implements functions specific to the StorageProviderNode
type FakeProviderNode struct {
	FakeCommonNode
	MinerAddr                           address.Address
	MinerWorkerError                    error
	PieceLength                         uint64
	PieceSectorID                       uint64
	PublishDealID                       abi.DealID
	PublishDealsError                   error
	OnDealCompleteError                 error
	LocatePieceForDealWithinSectorError error
	DealCommittedSyncError              error
	DealCommittedAsyncError             error
	SignBytesError                      error
}

// PublishDeals simulates publishing a deal by adding it to the storage market state
func (n *FakeProviderNode) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (abi.DealID, cid.Cid, error) {
	if n.PublishDealsError == nil {
		sd := storagemarket.StorageDeal{
			DealProposal: deal.Proposal,
			DealState:    market.DealState{},
		}

		n.SMState.AddDeal(sd)

		return n.PublishDealID, shared_testutil.GenerateCids(1)[0], nil
	}
	return abi.DealID(0), cid.Undef, n.PublishDealsError
}

// ListProviderDeals returns the deals in the storage market state
func (n *FakeProviderNode) ListProviderDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	return n.SMState.Deals(addr), nil
}

// OnDealComplete simulates passing of the deal to the storage miner, and does nothing
func (n *FakeProviderNode) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceReader io.Reader) error {
	return n.OnDealCompleteError
}

// GetMinerWorker returns the address specified by MinerAddr
func (n *FakeProviderNode) GetMinerWorker(ctx context.Context, miner address.Address) (address.Address, error) {
	if n.MinerWorkerError == nil {
		return n.MinerAddr, nil
	}
	return address.Undef, n.MinerWorkerError
}

// SignBytes simulates signing data by returning a test signature
func (n *FakeProviderNode) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	if n.SignBytesError == nil {
		return shared_testutil.MakeTestSignature(), nil
	}
	return nil, n.SignBytesError
}

// OnDealSectorCommitted returns immediately, with success
func (n *FakeProviderNode) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, cb storagemarket.DealSectorCommittedCallback) error {
	if n.DealCommittedSyncError == nil {
		cb(n.DealCommittedAsyncError)
	}
	return n.DealCommittedSyncError
}

// LocatePieceForDealWithinSector returns stubbed data for a pieces location in a sector
func (n *FakeProviderNode) LocatePieceForDealWithinSector(ctx context.Context, dealID abi.DealID) (sectorID uint64, offset uint64, length uint64, err error) {
	if n.LocatePieceForDealWithinSectorError == nil {
		return n.PieceSectorID, 0, n.PieceLength, nil
	}
	return 0, 0, 0, n.LocatePieceForDealWithinSectorError
}

var _ storagemarket.StorageProviderNode = (*FakeProviderNode)(nil)
