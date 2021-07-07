package storageimpl

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer"

	mktdagstore "github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// -------
// clientDealEnvironment
// -------

type clientDealEnvironment struct {
	c *Client
}

func (c *clientDealEnvironment) NewDealStream(ctx context.Context, p peer.ID) (network.StorageDealStream, error) {
	return c.c.net.NewDealStream(ctx, p)
}

func (c *clientDealEnvironment) Node() storagemarket.StorageClientNode {
	return c.c.node
}

func (c *clientDealEnvironment) CleanBlockstore(proposalCid cid.Cid) error {
	return c.c.readOnlyCARStoreTracker.CleanBlockstore(proposalCid.String())
}

func (c *clientDealEnvironment) StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID,
	error) {
	chid, err := c.c.dataTransfer.OpenPushDataChannel(ctx, to, voucher, baseCid, selector)
	return chid, err
}

func (c *clientDealEnvironment) RestartDataTransfer(ctx context.Context, channelId datatransfer.ChannelID) error {
	return c.c.dataTransfer.RestartDataTransferChannel(ctx, channelId)
}

func (c *clientDealEnvironment) GetProviderDealState(ctx context.Context, proposalCid cid.Cid) (*storagemarket.ProviderDealState, error) {
	return c.c.GetProviderDealState(ctx, proposalCid)
}

func (c *clientDealEnvironment) PollingInterval() time.Duration {
	return c.c.pollingInterval
}

type clientStoreGetter struct {
	c *Client
}

func (csg *clientStoreGetter) Get(proposalCid cid.Cid) (bstore.Blockstore, error) {
	var deal storagemarket.ClientDeal
	err := csg.c.statemachines.Get(proposalCid).Get(&deal)
	if err != nil {
		return nil, xerrors.Errorf("failed to get client deal state, err=%w", err)
	}

	// get a read Only CARv2 blockstore that provides random access on top of
	// the client's CARv2 file containing the CARv1 payload that needs to be
	// transferred as part of the deal.
	bs, err := csg.c.readOnlyCARStoreTracker.GetOrCreate(proposalCid.String(), deal.CARv2FilePath)
	if err != nil {
		return nil, err
	}

	// Wrap the ReadBlockstore with a Blockstore implementation that panics
	// if any write methods are called (eg Put, Delete).
	// We need to do this because the Get interface returns a
	// blockstore.Blockstore, however we know that for the storage market
	// client we will only ever be doing reads from the blockstore.
	return mktdagstore.NewReadOnlyBlockstore(bs), err
}

func (c *clientDealEnvironment) TagPeer(peer peer.ID, tag string) {
	c.c.net.TagPeer(peer, tag)
}

func (c *clientDealEnvironment) UntagPeer(peer peer.ID, tag string) {
	c.c.net.UntagPeer(peer, tag)
}

type clientPullDeals struct {
	c *Client
}

func (cpd *clientPullDeals) Get(proposalCid cid.Cid) (storagemarket.ClientDeal, error) {
	var deal storagemarket.ClientDeal
	err := cpd.c.statemachines.GetSync(context.TODO(), proposalCid, &deal)
	return deal, err
}
