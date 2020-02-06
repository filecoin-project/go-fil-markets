package storageimpl

// this file implements storagemarket.StorageClient

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

func (c *Client) ListProviders(ctx context.Context) (<-chan storagemarket.StorageProviderInfo, error) {
	providers, err := c.node.ListStorageProviders(ctx)
	if err != nil {
		return nil, err
	}

	out := make(chan storagemarket.StorageProviderInfo)

	go func() {
		defer close(out)
		for _, p := range providers {
			select {
			case out <- *p:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (c *Client) ListDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	return c.node.ListClientDeals(ctx, addr)
}

func (c *Client) ListInProgressDeals(ctx context.Context) ([]storagemarket.ClientDeal, error) {
	deals, err := c.List()
	if err != nil {
		return nil, err
	}

	out := make([]storagemarket.ClientDeal, len(deals))
	for k, v := range deals {
		out[k] = storagemarket.ClientDeal{
			ProposalCid:    v.ProposalCid,
			Proposal:       v.Proposal,
			State:          v.State,
			Miner:          v.Miner,
			MinerWorker:    v.MinerWorker,
			DealID:         v.DealID,
			PublishMessage: v.PublishMessage,
		}
	}

	return out, nil
}

func (c *Client) GetInProgressDeal(ctx context.Context, cid cid.Cid) (storagemarket.ClientDeal, error) {
	deals, err := c.ListInProgressDeals(ctx)
	if err != nil {
		return storagemarket.ClientDeal{}, err
	}

	for _, deal := range deals {
		if deal.ProposalCid == cid {
			return deal, nil
		}
	}

	return storagemarket.ClientDeal{}, xerrors.Errorf("couldn't find client deal")
}

func (c *Client) GetAsk(ctx context.Context, info storagemarket.StorageProviderInfo) (*storagemarket.SignedStorageAsk, error) {
	return c.QueryAsk(ctx, info.PeerID, info.Address)
}

func (c *Client) ProposeStorageDeal(
	ctx context.Context,
	addr address.Address,
	info *storagemarket.StorageProviderInfo,
	data *storagemarket.DataRef,
	proposalExpiration storagemarket.Epoch,
	duration storagemarket.Epoch,
	price abi.TokenAmount,
	collateral abi.TokenAmount,
) (*storagemarket.ProposeStorageDealResult, error) {

	proposal := ClientDealProposal{
		Data:               data,
		PricePerEpoch:      price,
		ProposalExpiration: uint64(proposalExpiration),
		Duration:           uint64(duration),
		Client:             addr,
		ProviderAddress:    info.Address,
		MinerWorker:        info.Worker,
		MinerID:            info.PeerID,
	}

	proposalCid, err := c.Start(ctx, proposal)

	result := &storagemarket.ProposeStorageDealResult{
		ProposalCid: proposalCid,
	}

	return result, err
}

func (c *Client) GetPaymentEscrow(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {

	balance, err := c.node.GetBalance(ctx, addr)

	return balance, err
}

func (c *Client) AddPaymentEscrow(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {

	return c.node.AddFunds(ctx, addr, amount)
}

var _ storagemarket.StorageClient = &Client{}
