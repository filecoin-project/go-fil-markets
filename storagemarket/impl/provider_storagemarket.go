package storageimpl

// this file implements storagemarket.StorageClient

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

func (p *Provider) AddAsk(price abi.TokenAmount, duration abi.ChainEpoch) error {
	return p.SetPrice(price, duration)
}

func (p *Provider) ListAsks(addr address.Address) []*storagemarket.SignedStorageAsk {
	ask := p.GetAsk(addr)

	if ask != nil {
		return []*storagemarket.SignedStorageAsk{ask}
	}

	return nil
}

func (p *Provider) ListDeals(ctx context.Context) ([]storagemarket.StorageDeal, error) {
	return p.spn.ListProviderDeals(ctx, p.actor)
}

func (p *Provider) AddStorageCollateral(ctx context.Context, amount abi.TokenAmount) error {
	return p.spn.AddFunds(ctx, p.actor, amount)
}

func (p *Provider) GetStorageCollateral(ctx context.Context) (storagemarket.Balance, error) {
	balance, err := p.spn.GetBalance(ctx, p.actor)

	return balance, err
}

func (p *Provider) ListIncompleteDeals() ([]storagemarket.MinerDeal, error) {
	var out []storagemarket.MinerDeal

	var deals []MinerDeal
	if err := p.deals.List(&deals); err != nil {
		return nil, err
	}

	for _, deal := range deals {
		out = append(out, storagemarket.MinerDeal{
			Client:             deal.Client,
			ClientDealProposal: deal.ClientDealProposal,
			ProposalCid:        deal.ProposalCid,
			State:              deal.State,
			Ref:                deal.Ref,
			DealID:             deal.DealID,
		})
	}

	return out, nil
}

var _ storagemarket.StorageProvider = &Provider{}
