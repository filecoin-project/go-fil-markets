package migrations

import (
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/filecoin-project/go-ds-versioning/pkg/versioned"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

//go:generate cbor-gen-for ClientDeal0 MinerDeal0 Balance0 SignedStorageAsk0 StorageAsk0 DataRef0 ProviderDealState0

// Balance0 is version 0 of Balance
type Balance0 struct {
	Locked    abi.TokenAmount
	Available abi.TokenAmount
}

// StorageAsk0 is version 0 of StorageAsk
type StorageAsk0 struct {
	Price         abi.TokenAmount
	VerifiedPrice abi.TokenAmount

	MinPieceSize abi.PaddedPieceSize
	MaxPieceSize abi.PaddedPieceSize
	Miner        address.Address
	Timestamp    abi.ChainEpoch
	Expiry       abi.ChainEpoch
	SeqNo        uint64
}

// SignedStorageAsk0 is version 0 of SignedStorageAsk
type SignedStorageAsk0 struct {
	Ask       *StorageAsk0
	Signature *crypto.Signature
}

// MinerDeal0 is version 0 of MinerDeal
type MinerDeal0 struct {
	market.ClientDealProposal
	ProposalCid           cid.Cid
	AddFundsCid           *cid.Cid
	PublishCid            *cid.Cid
	Miner                 peer.ID
	Client                peer.ID
	State                 storagemarket.StorageDealStatus
	PiecePath             filestore.Path
	MetadataPath          filestore.Path
	SlashEpoch            abi.ChainEpoch
	FastRetrieval         bool
	Message               string
	StoreID               *multistore.StoreID
	FundsReserved         abi.TokenAmount
	Ref                   *DataRef0
	AvailableForRetrieval bool

	DealID       abi.DealID
	CreationTime cbg.CborTime
}

// ClientDeal0 is version 0 of ClientDeal
type ClientDeal0 struct {
	market.ClientDealProposal
	ProposalCid    cid.Cid
	AddFundsCid    *cid.Cid
	State          storagemarket.StorageDealStatus
	Miner          peer.ID
	MinerWorker    address.Address
	DealID         abi.DealID
	DataRef        *DataRef0
	Message        string
	PublishMessage *cid.Cid
	SlashEpoch     abi.ChainEpoch
	PollRetryCount uint64
	PollErrorCount uint64
	FastRetrieval  bool
	StoreID        *multistore.StoreID
	FundsReserved  abi.TokenAmount
	CreationTime   cbg.CborTime
}

// DataRef0 is version 0 of DataRef
type DataRef0 struct {
	TransferType string
	Root         cid.Cid
	PieceCid     *cid.Cid
	PieceSize    abi.UnpaddedPieceSize
}

// ProviderDealState0 is version 0 of ProviderDealState
type ProviderDealState0 struct {
	State         storagemarket.StorageDealStatus
	Message       string
	Proposal      *market.DealProposal
	ProposalCid   *cid.Cid
	AddFundsCid   *cid.Cid
	PublishCid    *cid.Cid
	DealID        abi.DealID
	FastRetrieval bool
}

// MigrateDataRef0To1 migrates a tuple encoded data tref to a map encoded data ref
func MigrateDataRef0To1(oldDr *DataRef0) *storagemarket.DataRef {
	if oldDr == nil {
		return nil
	}
	return &storagemarket.DataRef{
		TransferType: oldDr.TransferType,
		Root:         oldDr.Root,
		PieceCid:     oldDr.PieceCid,
		PieceSize:    oldDr.PieceSize,
	}
}

// MigrateClientDeal0To1 migrates a tuple encoded client deal to a map encoded client deal
func MigrateClientDeal0To1(oldCd *ClientDeal0) (*storagemarket.ClientDeal, error) {
	return &storagemarket.ClientDeal{
		ClientDealProposal: oldCd.ClientDealProposal,
		ProposalCid:        oldCd.ProposalCid,
		AddFundsCid:        oldCd.AddFundsCid,
		State:              oldCd.State,
		Miner:              oldCd.Miner,
		MinerWorker:        oldCd.MinerWorker,
		DealID:             oldCd.DealID,
		DataRef:            MigrateDataRef0To1(oldCd.DataRef),
		Message:            oldCd.Message,
		PublishMessage:     oldCd.PublishMessage,
		SlashEpoch:         oldCd.SlashEpoch,
		PollRetryCount:     oldCd.PollRetryCount,
		PollErrorCount:     oldCd.PollErrorCount,
		FastRetrieval:      oldCd.FastRetrieval,
		StoreID:            oldCd.StoreID,
		FundsReserved:      oldCd.FundsReserved,
		CreationTime:       oldCd.CreationTime,
	}, nil
}

// MigrateMinerDeal0To1 migrates a tuple encoded miner deal to a map encoded miner deal
func MigrateMinerDeal0To1(oldCd *MinerDeal0) (*storagemarket.MinerDeal, error) {
	return &storagemarket.MinerDeal{
		ClientDealProposal:    oldCd.ClientDealProposal,
		ProposalCid:           oldCd.ProposalCid,
		AddFundsCid:           oldCd.AddFundsCid,
		PublishCid:            oldCd.PublishCid,
		Miner:                 oldCd.Miner,
		Client:                oldCd.Client,
		State:                 oldCd.State,
		PiecePath:             oldCd.PiecePath,
		MetadataPath:          oldCd.MetadataPath,
		SlashEpoch:            oldCd.SlashEpoch,
		FastRetrieval:         oldCd.FastRetrieval,
		Message:               oldCd.Message,
		StoreID:               oldCd.StoreID,
		FundsReserved:         oldCd.FundsReserved,
		Ref:                   MigrateDataRef0To1(oldCd.Ref),
		AvailableForRetrieval: oldCd.AvailableForRetrieval,
		DealID:                oldCd.DealID,
		CreationTime:          oldCd.CreationTime,
	}, nil
}

// ClientMigrations are migrations for the client's store of storage deals
var ClientMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateClientDeal0To1, versioning.VersionKey("1")),
}

// ProviderMigrations are migrations for the providers's store of storage deals
var ProviderMigrations = versioned.BuilderList{
	versioned.NewVersionedBuilder(MigrateMinerDeal0To1, versioning.VersionKey("1")),
}
