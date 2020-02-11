package storagemarket

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

//go:generate cbor-gen-for ClientDeal MinerDeal Balance SignedStorageAsk StorageAsk StorageDeal DataRef

const DealProtocolID = "/fil/storage/mk/1.0.1"
const AskProtocolID = "/fil/storage/ask/1.0.1"

type Balance struct {
	Locked    abi.TokenAmount
	Available abi.TokenAmount
}

type StorageDealStatus = uint64

const (
	StorageDealUnknown = StorageDealStatus(iota)
	StorageDealProposalNotFound
	StorageDealProposalRejected
	StorageDealProposalAccepted
	StorageDealStaged
	StorageDealSealing
	StorageDealProposalSigned
	StorageDealPublished
	StorageDealCommitted
	StorageDealActive
	StorageDealFailing
	StorageDealRecovering
	StorageDealExpired
	StorageDealNotFound

	// Internal

	StorageDealValidating   // Verifying that deal parameters are good
	StorageDealTransferring // Moving data
	StorageDealVerifyData   // Verify transferred data - generate CAR / piece data
	StorageDealPublishing   // Publishing deal to chain
	StorageDealError        // deal failed with an unexpected error

	StorageDealNoUpdate = StorageDealUnknown
)

var DealStates = []string{
	"StorageDealUnknown",
	"StorageDealProposalNotFound",
	"StorageDealProposalRejected",
	"StorageDealProposalAccepted",
	"StorageDealStaged",
	"StorageDealSealing",
	"StorageDealProposalSigned",
	"StorageDealPublished",
	"StorageDealCommitted",
	"StorageDealActive",
	"StorageDealFailing",
	"StorageDealRecovering",
	"StorageDealExpired",
	"StorageDealNotFound",

	"StorageDealValidating",
	"StorageDealTransferring",
	"StorageDealVerifyData",
	"StorageDealPublishing",
	"StorageDealError",
}

type DealID uint64

func init() {
	cbor.RegisterCborType(SignedStorageAsk{})
	cbor.RegisterCborType(StorageAsk{})
}

type SignedStorageAsk struct {
	Ask       *StorageAsk
	Signature *crypto.Signature
}

type StorageAsk struct {
	// Price per GiB / Epoch
	Price abi.TokenAmount

	MinPieceSize abi.PaddedPieceSize
	Miner        address.Address
	Timestamp    abi.ChainEpoch
	Expiry       abi.ChainEpoch
	SeqNo        uint64
}

type StateKey interface {
	Height() abi.ChainEpoch
}

// Duplicated from deals package for now
type MinerDeal struct {
	market.ClientDealProposal
	ProposalCid cid.Cid
	Miner       peer.ID
	Client      peer.ID
	State       StorageDealStatus
	PiecePath   filestore.Path

	Ref *DataRef

	DealID uint64
}

type ClientDeal struct {
	market.ClientDealProposal
	ProposalCid cid.Cid
	State       StorageDealStatus
	Miner       peer.ID
	MinerWorker address.Address
	DealID      uint64
	DataRef     *DataRef

	PublishMessage *cid.Cid
}

// StorageDeal is a local combination of a proposal and a current deal state
type StorageDeal struct {
	market.DealProposal
	market.DealState
}

// StorageProvider is the interface provided for storage providers
type StorageProvider interface {
	Start(ctx context.Context) error

	Stop() error

	AddAsk(price abi.TokenAmount, duration abi.ChainEpoch) error

	// ListAsks lists current asks
	ListAsks(addr address.Address) []*SignedStorageAsk

	// ListDeals lists on-chain deals associated with this provider
	ListDeals(ctx context.Context) ([]StorageDeal, error)

	// ListIncompleteDeals lists deals that are in progress or rejected
	ListIncompleteDeals() ([]MinerDeal, error)

	// AddStorageCollateral adds storage collateral
	AddStorageCollateral(ctx context.Context, amount abi.TokenAmount) error

	// GetStorageCollateral returns the current collateral balance
	GetStorageCollateral(ctx context.Context) (Balance, error)

	ImportDataForDeal(ctx context.Context, propCid cid.Cid, data io.Reader) error
}

// Node dependencies for a StorageProvider
type StorageProviderNode interface {
	MostRecentStateId(ctx context.Context) (StateKey, error)

	// Verify a signature against an address + data
	VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) bool

	// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
	AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error

	// Ensures that a storage market participant has a certain amount of available funds
	EnsureFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error

	// GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
	GetBalance(ctx context.Context, addr address.Address) (Balance, error)

	// Publishes deal on chain
	PublishDeals(ctx context.Context, deal MinerDeal) (DealID, cid.Cid, error)

	// ListProviderDeals lists all deals associated with a storage provider
	ListProviderDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// Called when a deal is complete and on chain, and data has been transferred and is ready to be added to a sector
	OnDealComplete(ctx context.Context, deal MinerDeal, pieceSize uint64, pieceReader io.Reader) error

	// returns the worker address associated with a miner
	GetMinerWorker(ctx context.Context, miner address.Address) (address.Address, error)

	// Signs bytes
	SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error)

	OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID uint64, cb DealSectorCommittedCallback) error

	LocatePieceForDealWithinSector(ctx context.Context, dealID uint64) (sectorID uint64, offset uint64, length uint64, err error)
}

type DealSectorCommittedCallback func(err error)

// Node dependencies for a StorageClient
type StorageClientNode interface {
	MostRecentStateId(ctx context.Context) (StateKey, error)

	// Verify a signature against an address + data
	VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) bool

	// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
	AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error

	EnsureFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error

	// GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
	GetBalance(ctx context.Context, addr address.Address) (Balance, error)

	//// ListClientDeals lists all on-chain deals associated with a storage client
	ListClientDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// GetProviderInfo returns information about a single storage provider
	//GetProviderInfo(stateId StateID, addr Address) *StorageProviderInfo

	// GetStorageProviders returns information about known miners
	ListStorageProviders(ctx context.Context) ([]*StorageProviderInfo, error)

	// Subscribes to storage market actor state changes for a given address.
	// TODO: Should there be a timeout option for this?  In the case that we are waiting for funds to be deposited and it never happens?
	//SubscribeStorageMarketEvents(addr Address, handler StorageMarketEventHandler) (SubID, error)

	// Cancels a subscription
	//UnsubscribeStorageMarketEvents(subId SubID)
	ValidatePublishedDeal(ctx context.Context, deal ClientDeal) (uint64, error)

	// SignProposal signs a proposal
	SignProposal(ctx context.Context, signer address.Address, proposal market.DealProposal) (*market.ClientDealProposal, error)

	GetDefaultWalletAddress(ctx context.Context) (address.Address, error)

	OnDealSectorCommitted(ctx context.Context, provider address.Address, dealId uint64, cb DealSectorCommittedCallback) error

	ValidateAskSignature(ask *SignedStorageAsk) error
}

type StorageClientProofs interface {
	//GeneratePieceCommitment(piece io.Reader, pieceSize uint64) (CommP, error)
}

// Closely follows the MinerInfo struct in the spec
type StorageProviderInfo struct {
	Address    address.Address // actor address
	Owner      address.Address
	Worker     address.Address // signs messages
	SectorSize uint64
	PeerID     peer.ID
	// probably more like how much storage power, available collateral etc
}

type ProposeStorageDealResult struct {
	ProposalCid cid.Cid
}

const (
	TTGraphsync = "graphsync"
	TTManual    = "manual"
)

type DataRef struct {
	TransferType string
	Root         cid.Cid
}

// The interface provided by the module to the outside world for storage clients.
type StorageClient interface {
	Run(ctx context.Context)

	Stop()

	// ListProviders queries chain state and returns active storage providers
	ListProviders(ctx context.Context) (<-chan StorageProviderInfo, error)

	// ListDeals lists on-chain deals associated with this provider
	ListDeals(ctx context.Context, addr address.Address) ([]StorageDeal, error)

	// ListInProgressDeals lists deals that are in progress or rejected
	ListInProgressDeals(ctx context.Context) ([]ClientDeal, error)

	// ListInProgressDeals lists deals that are in progress or rejected
	GetInProgressDeal(ctx context.Context, cid cid.Cid) (ClientDeal, error)

	// GetAsk returns the current ask for a storage provider
	GetAsk(ctx context.Context, info StorageProviderInfo) (*SignedStorageAsk, error)

	//// FindStorageOffers lists providers and queries them to find offers that satisfy some criteria based on price, duration, etc.
	//FindStorageOffers(criteria AskCriteria, limit uint) []*StorageOffer

	// ProposeStorageDeal initiates deal negotiation with a Storage Provider
	ProposeStorageDeal(ctx context.Context, addr address.Address, info *StorageProviderInfo, data *DataRef, startEpoch abi.ChainEpoch, endEpoch abi.ChainEpoch, price abi.TokenAmount, collateral abi.TokenAmount) (*ProposeStorageDealResult, error)

	// GetPaymentEscrow returns the current funds available for deal payment
	GetPaymentEscrow(ctx context.Context, addr address.Address) (Balance, error)

	// AddStorageCollateral adds storage collateral
	AddPaymentEscrow(ctx context.Context, addr address.Address, amount abi.TokenAmount) error
}
