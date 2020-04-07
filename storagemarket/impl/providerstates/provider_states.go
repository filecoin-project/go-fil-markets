package providerstates

import (
	"context"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

var log = logging.Logger("providerstates")

// ProviderDealEnvironment are the dependencies needed for processing deals
// with a ProviderStateEntryFunc
type ProviderDealEnvironment interface {
	Address() address.Address
	Node() storagemarket.StorageProviderNode
	Ask() storagemarket.StorageAsk
	StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error
	GeneratePieceCommitmentToFile(payloadCid cid.Cid, selector ipld.Node) (cid.Cid, filestore.Path, filestore.Path, error)
	SendSignedResponse(ctx context.Context, response *network.Response) error
	Disconnect(proposalCid cid.Cid) error
	FileStore() filestore.FileStore
	PieceStore() piecestore.PieceStore
	DealAcceptanceBuffer() abi.ChainEpoch
}

// ProviderStateEntryFunc is the signature for a StateEntryFunc in the provider FSM
type ProviderStateEntryFunc func(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error

// ValidateDealProposal validates a proposed deal against the provider criteria
func ValidateDealProposal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("getting most recent state id: %w", err))
	}

	if err := providerutils.VerifyProposal(ctx.Context(), deal.ClientDealProposal, tok, environment.Node().VerifySignature); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("verifying StorageDealProposal: %w", err))
	}

	if deal.Proposal.Provider != environment.Address() {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("incorrect provider for deal"))
	}

	tok, height, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("getting most recent state id: %w", err))
	}

	if height > deal.Proposal.StartEpoch-environment.DealAcceptanceBuffer() {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("deal start epoch is too soon or deal already expired"))
	}

	// TODO: check StorageCollateral

	minPrice := big.Div(big.Mul(environment.Ask().Price, abi.NewTokenAmount(int64(deal.Proposal.PieceSize))), abi.NewTokenAmount(1<<30))
	if deal.Proposal.StoragePricePerEpoch.LessThan(minPrice) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("storage price per epoch less than asking price: %s < %s", deal.Proposal.StoragePricePerEpoch, minPrice))
	}

	if deal.Proposal.PieceSize < environment.Ask().MinPieceSize {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("piece size less than minimum required size: %d < %d", deal.Proposal.PieceSize, environment.Ask().MinPieceSize))
	}

	if deal.Proposal.PieceSize > environment.Ask().MaxPieceSize {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("piece size more than maximum allowed size: %d > %d", deal.Proposal.PieceSize, environment.Ask().MaxPieceSize))
	}

	// check market funds
	clientMarketBalance, err := environment.Node().GetBalance(ctx.Context(), deal.Proposal.Client, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("getting client market balance failed: %w", err))
	}

	// This doesn't guarantee that the client won't withdraw / lock those funds
	// but it's a decent first filter
	if clientMarketBalance.Available.LessThan(deal.Proposal.TotalStorageFee()) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.New("clientMarketBalance.Available too small"))
	}

	// TODO: Send intent to accept
	return ctx.Trigger(storagemarket.ProviderEventDealAccepted)
}

// TransferData initiates a data transfer or places the deal in a waiting state if it is a
// manual deal
func TransferData(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	if deal.Ref.TransferType == storagemarket.TTManual {
		log.Info("deal entering manual transfer state")
		return ctx.Trigger(storagemarket.ProviderEventWaitingForManualData)
	}

	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	// this is the selector for "get the whole DAG"
	// TODO: support storage deals with custom payload selectors
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	log.Infof("fetching data for a deal %s", deal.ProposalCid)

	// initiate a pull data transfer. This will complete asynchronously and the
	// completion of the data transfer will trigger a change in deal state
	// (see onDataTransferEvent)
	err := environment.StartDataTransfer(ctx.Context(),
		deal.Client,
		&requestvalidation.StorageDataTransferVoucher{Proposal: deal.ProposalCid},
		deal.Ref.Root,
		allSelector,
	)

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDataTransferFailed, xerrors.Errorf("failed to open pull data channel: %w", err))
	}

	return ctx.Trigger(storagemarket.ProviderEventDataTransferInitiated)
}

// VerifyData verifies that data received for a deal matches the pieceCID
// in the proposal
func VerifyData(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// entire DAG selector
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	pieceCid, piecePath, metadataPath, err := environment.GeneratePieceCommitmentToFile(deal.Ref.Root, allSelector)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventGeneratePieceCIDFailed, err)
	}

	// Verify CommP matches
	if pieceCid != deal.Proposal.PieceCID {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal CommP doesn't match calculated CommP"))
	}

	return ctx.Trigger(storagemarket.ProviderEventVerifiedData, piecePath, metadataPath)
}

// PublishDeal publishes a deal on chain and sends the deal id back to the client
func PublishDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("acquiring chain head: %w", err))
	}

	waddr, err := environment.Node().GetMinerWorkerAddress(ctx.Context(), deal.Proposal.Provider, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("looking up miner worker: %w", err))
	}

	// TODO: check StorageCollateral (may be too large (or too small))
	if err := environment.Node().EnsureFunds(ctx.Context(), deal.Proposal.Provider, waddr, deal.Proposal.ProviderCollateral, tok); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("ensuring funds: %w", err))
	}

	smDeal := storagemarket.MinerDeal{
		Client:             deal.Client,
		ClientDealProposal: deal.ClientDealProposal,
		ProposalCid:        deal.ProposalCid,
		State:              deal.State,
		Ref:                deal.Ref,
	}

	dealID, mcid, err := environment.Node().PublishDeals(ctx.Context(), smDeal)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("publishing deal: %w", err))
	}

	err = environment.SendSignedResponse(ctx.Context(), &network.Response{
		State: storagemarket.StorageDealProposalAccepted,

		Proposal:       deal.ProposalCid,
		PublishMessage: &mcid,
	})

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventSendResponseFailed, err)
	}

	if err := environment.Disconnect(deal.ProposalCid); err != nil {
		log.Warnf("closing client connection: %+v", err)
	}

	return ctx.Trigger(storagemarket.ProviderEventDealPublished, dealID)
}

// HandoffDeal hands off a published deal for sealing and commitment in a sector
func HandoffDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	file, err := environment.FileStore().Open(deal.PiecePath)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventFileStoreErrored, xerrors.Errorf("reading piece at path %s: %w", deal.PiecePath, err))
	}
	paddedReader, paddedSize := padreader.New(file, uint64(file.Size()))
	err = environment.Node().OnDealComplete(
		ctx.Context(),
		storagemarket.MinerDeal{
			Client:             deal.Client,
			ClientDealProposal: deal.ClientDealProposal,
			ProposalCid:        deal.ProposalCid,
			State:              deal.State,
			Ref:                deal.Ref,
			DealID:             deal.DealID,
		},
		paddedSize,
		paddedReader,
	)

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, err)
	}
	return ctx.Trigger(storagemarket.ProviderEventDealHandedOff)
}

// VerifyDealActivated verifies that a deal has been committed to a sector and activated
func VerifyDealActivated(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// TODO: consider waiting for seal to happen
	cb := func(err error) {
		if err != nil {
			_ = ctx.Trigger(storagemarket.ProviderEventDealActivationFailed, err)
		} else {
			_ = ctx.Trigger(storagemarket.ProviderEventDealActivated)
		}
	}

	err := environment.Node().OnDealSectorCommitted(ctx.Context(), deal.Proposal.Provider, deal.DealID, cb)

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealActivationFailed, err)
	}
	return nil
}

// RecordPieceInfo records sector information about an activated deal so that the data
// can be retrieved later
func RecordPieceInfo(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventUnableToLocatePiece, deal.DealID, err)
	}

	sectorID, offset, length, err := environment.Node().LocatePieceForDealWithinSector(ctx.Context(), deal.DealID, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventUnableToLocatePiece, deal.DealID, err)
	}

	var blockLocations map[cid.Cid]piecestore.BlockLocation
	if deal.MetadataPath != filestore.Path("") {
		blockLocations, err = providerutils.LoadBlockLocations(environment.FileStore(), deal.MetadataPath)
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventReadMetadataErrored, err)
		}
	} else {
		blockLocations = map[cid.Cid]piecestore.BlockLocation{
			deal.Ref.Root: {},
		}
	}

	// TODO: Record actual block locations for all CIDs in piece by improving car writing
	err = environment.PieceStore().AddPieceBlockLocations(deal.Proposal.PieceCID, blockLocations)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, xerrors.Errorf("adding piece block locations: %w", err))
	}

	err = environment.PieceStore().AddDealForPiece(deal.Proposal.PieceCID, piecestore.DealInfo{
		DealID:   deal.DealID,
		SectorID: sectorID,
		Offset:   offset,
		Length:   length,
	})

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, xerrors.Errorf("adding deal info for piece: %w", err))
	}

	err = environment.FileStore().Delete(deal.PiecePath)
	if err != nil {
		log.Warnf("deleting piece at path %s: %w", deal.PiecePath, err)
	}
	if deal.MetadataPath != filestore.Path("") {
		err := environment.FileStore().Delete(deal.MetadataPath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.MetadataPath, err)
		}
	}

	return ctx.Trigger(storagemarket.ProviderEventDealCompleted)
}

// FailDeal sends a failure response before terminating a deal
func FailDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {

	log.Warnf("deal %s failed: %s", deal.ProposalCid, deal.Message)

	if !deal.ConnectionClosed {
		err := environment.SendSignedResponse(ctx.Context(), &network.Response{
			State:    storagemarket.StorageDealFailing,
			Message:  deal.Message,
			Proposal: deal.ProposalCid,
		})

		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventSendResponseFailed, err)
		}

		if err := environment.Disconnect(deal.ProposalCid); err != nil {
			log.Warnf("closing client connection: %+v", err)
		}
	}

	if deal.PiecePath != filestore.Path("") {
		err := environment.FileStore().Delete(deal.PiecePath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.PiecePath, err)
		}
	}
	if deal.MetadataPath != filestore.Path("") {
		err := environment.FileStore().Delete(deal.MetadataPath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.MetadataPath, err)
		}
	}
	return ctx.Trigger(storagemarket.ProviderEventFailed)
}
