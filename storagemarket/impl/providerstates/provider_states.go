package providerstates

import (
	"context"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/prometheus/common/log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

// ProviderDealEnvironment are the dependencies needed for processing deals
// with a ProviderStateEntryFunc
type ProviderDealEnvironment interface {
	Address() address.Address
	Node() storagemarket.StorageProviderNode
	Ask() storagemarket.StorageAsk
	StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error
	GeneratePieceCommitmentToFile(payloadCid cid.Cid, selector ipld.Node) (cid.Cid, filestore.Path, error)
	SendSignedResponse(ctx context.Context, response *network.Response) error
	Disconnect(proposalCid cid.Cid) error
	OpenFile(path filestore.Path) (filestore.File, error)
	DeleteFile(path filestore.Path) error
	AddDealForPiece(pieceCID cid.Cid, dealInfo piecestore.DealInfo) error
	AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]piecestore.BlockLocation) error
}

// ProviderStateEntryFunc is the signature for a StateEntryFunc in the provider FSM
type ProviderStateEntryFunc func(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error

// ValidateDealProposal validates a proposed deal against the provider criteria
func ValidateDealProposal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {

	if err := providerutils.VerifyProposal(deal.ClientDealProposal, environment.Node().VerifySignature); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("verifying StorageDealProposal: %w", err))
	}

	if deal.Proposal.Provider != environment.Address() {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("incorrect provider for deal"))
	}

	head, err := environment.Node().MostRecentStateId(ctx.Context())

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("getting most recent state id: ", err))
	}

	// TODO: set configurable value for how many epochs in the future StartEpoch must be
	if head.Height() >= deal.Proposal.StartEpoch {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("deal proposal already expired"))
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

	// check market funds
	clientMarketBalance, err := environment.Node().GetBalance(ctx.Context(), deal.Proposal.Client)
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

	pieceCid, path, err := environment.GeneratePieceCommitmentToFile(deal.Ref.Root, allSelector)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventGeneratePieceCIDFailed, err)
	}

	// Verify CommP matches
	if pieceCid != deal.Proposal.PieceCID {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal CommP doesn't match calculated CommP"))
	}

	return ctx.Trigger(storagemarket.ProviderEventVerifiedData, path)
}

// PublishDeal publishes a deal on chain and sends the deal id back to the client
func PublishDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	waddr, err := environment.Node().GetMinerWorker(ctx.Context(), deal.Proposal.Provider)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("looking up miner worker: %w", err))
	}

	// TODO: check StorageCollateral (may be too large (or too small))
	if err := environment.Node().EnsureFunds(ctx.Context(), deal.Proposal.Provider, waddr, deal.Proposal.ProviderCollateral); err != nil {
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
	file, err := environment.OpenFile(deal.PiecePath)
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
	err := environment.DeleteFile(deal.PiecePath)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventFileStoreErrored, xerrors.Errorf("deleting piece at path %s: %w", deal.PiecePath, err))
	}

	sectorID, offset, length, err := environment.Node().LocatePieceForDealWithinSector(ctx.Context(), deal.DealID)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("locating piece for deal ID %d in sector: %w", deal.DealID, err))
	}
	// TODO: Record actual block locations for all CIDs in piece by improving car writing
	err = environment.AddPieceBlockLocations(deal.Proposal.PieceCID, map[cid.Cid]piecestore.BlockLocation{
		deal.Ref.Root: {},
	})
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, xerrors.Errorf("adding piece block locations: %w", err))
	}

	err = environment.AddDealForPiece(deal.Proposal.PieceCID, piecestore.DealInfo{
		DealID:   deal.DealID,
		SectorID: sectorID,
		Offset:   offset,
		Length:   length,
	})

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, xerrors.Errorf("adding deal info for piece: %w", err))
	}

	return nil
}

// FailDeal sends a failure response before terminating a deal
func FailDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {

	log.Warnf("deal %s failed: %s", deal.ProposalCid, deal.Message)

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

	return ctx.Trigger(storagemarket.ProviderEventFailed)
}
