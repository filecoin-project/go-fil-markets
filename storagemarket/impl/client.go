package storageimpl

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/connmanager"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

var log = logging.Logger("storagemarket_impl")

var _ storagemarket.StorageClient = &Client{}

type Client struct {
	net network.StorageMarketNetwork

	// dataTransfer
	// TODO: once the data transfer module is complete, the
	// client will listen to events on the data transfer module
	// Because we are using only a fake DAGService
	// implementation, there's no validation or events on the client side
	dataTransfer datatransfer.Manager
	bs           blockstore.Blockstore
	fs           filestore.FileStore
	pio          pieceio.PieceIO
	discovery    *discovery.Local

	node storagemarket.StorageClientNode

	statemachines fsm.Group
	conns         *connmanager.ConnManager
}

func NewClient(
	net network.StorageMarketNetwork,
	bs blockstore.Blockstore,
	dataTransfer datatransfer.Manager,
	discovery *discovery.Local,
	ds datastore.Batching,
	scn storagemarket.StorageClientNode,
) (*Client, error) {
	carIO := cario.NewCarIO()
	pio := pieceio.NewPieceIO(carIO, bs)

	c := &Client{
		net:          net,
		dataTransfer: dataTransfer,
		bs:           bs,
		pio:          pio,
		discovery:    discovery,
		node:         scn,

		conns: connmanager.NewConnManager(),
	}

	statemachines, err := fsm.New(ds, fsm.Parameters{
		Environment:     &clientDealEnvironment{c},
		StateType:       storagemarket.ClientDeal{},
		StateKeyField:   "State",
		Events:          clientstates.ClientEvents,
		StateEntryFuncs: clientstates.ClientStateEntryFuncs,
	})
	if err != nil {
		return nil, err
	}
	c.statemachines = statemachines
	return c, nil
}

func (c *Client) Run(ctx context.Context) {
}

func (c *Client) Stop() {
	_ = c.statemachines.Stop(context.TODO())
}

func (c *Client) ListProviders(ctx context.Context) (<-chan storagemarket.StorageProviderInfo, error) {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	providers, err := c.node.ListStorageProviders(ctx, tok)
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
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	return c.node.ListClientDeals(ctx, addr, tok)
}

func (c *Client) ListLocalDeals(ctx context.Context) ([]storagemarket.ClientDeal, error) {
	var out []storagemarket.ClientDeal
	if err := c.statemachines.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetLocalDeal(ctx context.Context, cid cid.Cid) (storagemarket.ClientDeal, error) {
	var out storagemarket.ClientDeal
	if err := c.statemachines.Get(cid).Get(&out); err != nil {
		return storagemarket.ClientDeal{}, err
	}
	return out, nil
}

func (c *Client) GetAsk(ctx context.Context, info storagemarket.StorageProviderInfo) (*storagemarket.SignedStorageAsk, error) {
	s, err := c.net.NewAskStream(info.PeerID)
	if err != nil {
		return nil, xerrors.Errorf("failed to open stream to miner: %w", err)
	}

	request := network.AskRequest{Miner: info.Address}
	if err := s.WriteAskRequest(request); err != nil {
		return nil, xerrors.Errorf("failed to send ask request: %w", err)
	}

	out, err := s.ReadAskResponse()
	if err != nil {
		return nil, xerrors.Errorf("failed to read ask response: %w", err)
	}

	if out.Ask == nil {
		return nil, xerrors.Errorf("got no ask back")
	}

	if out.Ask.Ask.Miner != info.Address {
		return nil, xerrors.Errorf("got back ask for wrong miner")
	}

	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return nil, err
	}

	isValid, err := c.node.ValidateAskSignature(ctx, out.Ask, tok)
	if err != nil {
		return nil, err
	}

	if !isValid {
		return nil, xerrors.Errorf("ask was not properly signed")
	}

	return out.Ask, nil
}

func (c *Client) ProposeStorageDeal(
	ctx context.Context,
	addr address.Address,
	info *storagemarket.StorageProviderInfo,
	data *storagemarket.DataRef,
	startEpoch abi.ChainEpoch,
	endEpoch abi.ChainEpoch,
	price abi.TokenAmount,
	collateral abi.TokenAmount,
	rt abi.RegisteredProof,
) (*storagemarket.ProposeStorageDealResult, error) {
	commP, pieceSize, err := clientutils.CommP(ctx, c.pio, rt, data)
	if err != nil {
		return nil, xerrors.Errorf("computing commP failed: %w", err)
	}

	if uint64(pieceSize.Padded()) > info.SectorSize {
		return nil, fmt.Errorf("cannot propose a deal whose piece size (%d) is greater than sector size (%d)", pieceSize.Padded(), info.SectorSize)
	}

	dealProposal := market.DealProposal{
		PieceCID:             commP,
		PieceSize:            pieceSize.Padded(),
		Client:               addr,
		Provider:             info.Address,
		StartEpoch:           startEpoch,
		EndEpoch:             endEpoch,
		StoragePricePerEpoch: price,
		ProviderCollateral:   abi.NewTokenAmount(int64(pieceSize)), // TODO: real calc
		ClientCollateral:     big.Zero(),
	}

	clientDealProposal, err := c.node.SignProposal(ctx, addr, dealProposal)
	if err != nil {
		return nil, xerrors.Errorf("signing deal proposal failed: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(clientDealProposal)
	if err != nil {
		return nil, xerrors.Errorf("getting proposal node failed: %w", err)
	}

	deal := &storagemarket.ClientDeal{
		ProposalCid:        proposalNd.Cid(),
		ClientDealProposal: *clientDealProposal,
		State:              storagemarket.StorageDealUnknown,
		Miner:              info.PeerID,
		MinerWorker:        info.Worker,
		DataRef:            data,
	}

	err = c.statemachines.Begin(proposalNd.Cid(), deal)
	if err != nil {
		return nil, xerrors.Errorf("setting up deal tracking: %w", err)
	}

	s, err := c.net.NewDealStream(info.PeerID)
	if err != nil {
		return nil, xerrors.Errorf("connecting to storage provider failed: %w", err)
	}
	err = c.conns.AddStream(deal.ProposalCid, s)
	if err != nil {
		return nil, err
	}

	err = c.statemachines.Send(deal.ProposalCid, storagemarket.ClientEventOpen)
	if err != nil {
		return nil, xerrors.Errorf("initializing state machine: %w", err)
	}

	return &storagemarket.ProposeStorageDealResult{
			ProposalCid: deal.ProposalCid,
		}, c.discovery.AddPeer(data.Root, retrievalmarket.RetrievalPeer{
			Address: dealProposal.Provider,
			ID:      deal.Miner,
		})
}

func (c *Client) GetPaymentEscrow(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	tok, _, err := c.node.GetChainHead(ctx)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return c.node.GetBalance(ctx, addr, tok)
}

func (c *Client) AddPaymentEscrow(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	return c.node.AddFunds(ctx, addr, amount)
}

type clientDealEnvironment struct {
	c *Client
}

func (c *clientDealEnvironment) Node() storagemarket.StorageClientNode {
	return c.c.node
}

func (c *clientDealEnvironment) DealStream(proposalCid cid.Cid) (network.StorageDealStream, error) {
	return c.c.conns.DealStream(proposalCid)
}

func (c *clientDealEnvironment) CloseStream(proposalCid cid.Cid) error {
	return c.c.conns.Disconnect(proposalCid)
}
