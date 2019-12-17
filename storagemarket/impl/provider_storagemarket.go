package storageimpl

// this file implements storagemarket.StorageClient

import (
	"context"

	"github.com/filecoin-project/go-fil-components/shared/address"
	"github.com/filecoin-project/go-fil-components/shared/tokenamount"
	"github.com/filecoin-project/go-fil-components/shared/types"
	"github.com/filecoin-project/go-fil-components/storagemarket"
)

func (p *Provider) AddAsk(price tokenamount.TokenAmount, ttlsecs int64) error {
	return p.SetPrice(price, ttlsecs)
}

func (p *Provider) ListAsks(addr address.Address) []*types.SignedStorageAsk {
	ask := p.GetAsk(addr)

	if ask != nil {
		return []*types.SignedStorageAsk{ask}
	}

	return nil
}

func (p *Provider) ListDeals(ctx context.Context) ([]storagemarket.StorageDeal, error) {
	return p.spn.ListProviderDeals(ctx, p.actor)
}

func (p *Provider) AddStorageCollateral(ctx context.Context, amount tokenamount.TokenAmount) error {
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
			Client:      deal.Client,
			Proposal:    deal.Proposal,
			ProposalCid: deal.ProposalCid,
			State:       deal.State,
			Ref:         deal.Ref,
			DealID:      deal.DealID,
			SectorID:    deal.SectorID,
		})
	}

	return out, nil
}

var _ storagemarket.StorageProvider = &Provider{}
