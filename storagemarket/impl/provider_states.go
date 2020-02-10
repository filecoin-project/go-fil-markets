package storageimpl

import (
	"context"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-padreader"
	"github.com/ipfs/go-cid"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

type providerHandlerFunc func(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error)

func (p *Provider) handle(ctx context.Context, deal MinerDeal, cb providerHandlerFunc, next storagemarket.StorageDealStatus) {
	go func() {
		mut, err := cb(ctx, deal)

		if err == nil && next == storagemarket.StorageDealNoUpdate {
			return
		}

		select {
		case p.updated <- minerDealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
			mut:      mut,
		}:
		case <-p.stop:
		}
	}()
}

// StorageDealValidating
func (p *Provider) validating(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	head, err := p.spn.MostRecentStateId(ctx)
	if err != nil {
		return nil, err
	}
	if head.Height() >= deal.Proposal.Proposal.StartEpoch {
		return nil, xerrors.Errorf("deal proposal already expired")
	}

	// TODO: check StorageCollateral

	minPrice := big.Div(big.Mul(p.ask.Ask.Price, abi.NewTokenAmount(int64(deal.Proposal.Proposal.PieceSize))), abi.NewTokenAmount(1<<30))
	if deal.Proposal.Proposal.StoragePricePerEpoch.LessThan(minPrice) {
		return nil, xerrors.Errorf("storage price per epoch less than asking price: %s < %s", deal.Proposal.Proposal.StoragePricePerEpoch, minPrice)
	}

	if deal.Proposal.Proposal.PieceSize < p.ask.Ask.MinPieceSize {
		return nil, xerrors.Errorf("piece size less than minimum required size: %d < %d", deal.Proposal.Proposal.PieceSize, p.ask.Ask.MinPieceSize)
	}

	// check market funds
	clientMarketBalance, err := p.spn.GetBalance(ctx, deal.Proposal.Proposal.Client)
	if err != nil {
		return nil, xerrors.Errorf("getting client market balance failed: %w", err)
	}

	// This doesn't guarantee that the client won't withdraw / lock those funds
	// but it's a decent first filter
	if clientMarketBalance.Available.LessThan(deal.Proposal.Proposal.TotalStorageFee()) {
		return nil, xerrors.New("clientMarketBalance.Available too small")
	}

	// TODO: Send intent to accept
	return nil, nil
}

// State: StorageDealTransferring
func (p *Provider) transferring(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	if deal.Ref.TransferType == storagemarket.TTManual {
		log.Info("deal entering manual transfer state")
		return nil, nil
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
	_, err := p.dataTransfer.OpenPullDataChannel(ctx,
		deal.Client,
		&StorageDataTransferVoucher{Proposal: deal.ProposalCid},
		deal.Ref.Root,
		allSelector,
	)
	if err != nil {
		return nil, xerrors.Errorf("failed to open pull data channel: %w", err)
	}

	return nil, nil
}

// State: StorageDealVerifyData
func (p *Provider) verifydata(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	// entire DAG selector
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	commp, path, _, err := p.pio.GeneratePieceCommitmentToFile(deal.Ref.Root, allSelector)
	if err != nil {
		return nil, err
	}

	pieceCid := commcid.PieceCommitmentV1ToCID(commp)
	// Verify CommP matches
	if !pieceCid.Equals(deal.Proposal.Proposal.PieceCID) {
		return nil, xerrors.Errorf("proposal CommP doesn't match calculated CommP")
	}

	return func(deal *MinerDeal) {
		deal.PiecePath = path
	}, nil
}

// State: StorageDealPublishing
func (p *Provider) publishing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	waddr, err := p.spn.GetMinerWorker(ctx, deal.Proposal.Proposal.Provider)
	if err != nil {
		return nil, err
	}

	// TODO: check StorageCollateral (may be too large (or too small))
	if err := p.spn.EnsureFunds(ctx, waddr, deal.Proposal.Proposal.ProviderCollateral); err != nil {
		return nil, err
	}

	smDeal := storagemarket.MinerDeal{
		Client:      deal.Client,
		Proposal:    deal.Proposal,
		ProposalCid: deal.ProposalCid,
		State:       deal.State,
		Ref:         deal.Ref,
	}

	dealId, mcid, err := p.spn.PublishDeals(ctx, smDeal)
	if err != nil {
		return nil, err
	}

	err = p.sendSignedResponse(ctx, &network.Response{
		State: storagemarket.StorageDealProposalAccepted,

		Proposal:       deal.ProposalCid,
		PublishMessage: &mcid,
	})
	if err != nil {
		return nil, err
	}

	if err := p.disconnect(deal); err != nil {
		log.Warnf("closing client connection: %+v", err)
	}

	return func(deal *MinerDeal) {
		deal.DealID = uint64(dealId)
	}, nil
}

// STAGED
func (p *Provider) staged(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	file, err := p.fs.Open(deal.PiecePath)
	if err != nil {
		return nil, err
	}
	paddedReader, paddedSize := padreader.New(file, uint64(file.Size()))
	err = p.spn.OnDealComplete(
		ctx,
		storagemarket.MinerDeal{
			Client:      deal.Client,
			Proposal:    deal.Proposal,
			ProposalCid: deal.ProposalCid,
			State:       deal.State,
			Ref:         deal.Ref,
			DealID:      deal.DealID,
		},
		paddedSize,
		paddedReader,
	)

	return nil, err
}

// SEALING

func (p *Provider) sealing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	// TODO: consider waiting for seal to happen
	cb := func(err error) {
		select {
		case p.updated <- minerDealUpdate{
			newState: storagemarket.StorageDealActive,
			id:       deal.ProposalCid,
			err:      err,
		}:
		case <-p.stop:
		}
	}

	err := p.spn.OnDealSectorCommitted(ctx, deal.Proposal.Proposal.Provider, deal.DealID, cb)

	return nil, err

}

func (p *Provider) complete(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	err := p.fs.Delete(deal.PiecePath)
	if err != nil {
		return nil, err
	}
	sectorID, offset, length, err := p.spn.LocatePieceForDealWithinSector(ctx, deal.DealID)
	if err != nil {
		return nil, err
	}
	// TODO: Record actual block locations for all CIDs in piece by improving car writing
	err = p.pieceStore.AddPieceBlockLocations(deal.Proposal.Proposal.PieceCID, map[cid.Cid]piecestore.BlockLocation{
		deal.Ref.Root: {},
	})
	if err != nil {
		return nil, err
	}
	return nil, p.pieceStore.AddDealForPiece(deal.Proposal.Proposal.PieceCID, piecestore.DealInfo{
		DealID:   deal.DealID,
		SectorID: sectorID,
		Offset:   offset,
		Length:   length,
	})
}
