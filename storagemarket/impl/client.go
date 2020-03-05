package storageimpl

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-statemachine/fsm"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

//go:generate cbor-gen-for ClientDealProposal

var log = logging.Logger("storagemarket_impl")

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

	connsLk sync.RWMutex
	conns   map[cid.Cid]network.StorageDealStream
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

		conns: map[cid.Cid]network.StorageDealStream{},
	}

	statemachines, err := fsm.New(ds, fsm.Parameters{
		Environment:     c,
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

type ClientDealProposal struct {
	Data *storagemarket.DataRef

	PricePerEpoch abi.TokenAmount
	StartEpoch    abi.ChainEpoch
	EndEpoch      abi.ChainEpoch

	ProviderAddress address.Address
	ProofType       abi.RegisteredProof
	Client          address.Address
	MinerWorker     address.Address
	MinerID         peer.ID
}

func (c *Client) Start(ctx context.Context, p ClientDealProposal) (cid.Cid, error) {
	commP, pieceSize, err := clientutils.CommP(ctx, c.pio, p.ProofType, p.Data)
	if err != nil {
		return cid.Undef, xerrors.Errorf("computing commP failed: %w", err)
	}

	dealProposal := market.DealProposal{
		PieceCID:             commP,
		PieceSize:            pieceSize.Padded(),
		Client:               p.Client,
		Provider:             p.ProviderAddress,
		StartEpoch:           p.StartEpoch,
		EndEpoch:             p.EndEpoch,
		StoragePricePerEpoch: p.PricePerEpoch,
		ProviderCollateral:   abi.NewTokenAmount(int64(pieceSize)), // TODO: real calc
		ClientCollateral:     big.Zero(),
	}

	clientDealProposal, err := c.node.SignProposal(ctx, p.Client, dealProposal)
	if err != nil {
		return cid.Undef, xerrors.Errorf("signing deal proposal failed: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(clientDealProposal)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting proposal node failed: %w", err)
	}

	deal := &storagemarket.ClientDeal{
		ProposalCid:        proposalNd.Cid(),
		ClientDealProposal: *clientDealProposal,
		State:              storagemarket.StorageDealUnknown,
		Miner:              p.MinerID,
		MinerWorker:        p.MinerWorker,
		DataRef:            p.Data,
	}

	err = c.statemachines.Begin(proposalNd.Cid(), deal)
	if err != nil {
		return cid.Undef, xerrors.Errorf("setting up deal tracking: %w", err)
	}

	s, err := c.net.NewDealStream(p.MinerID)
	if err != nil {
		return cid.Undef, xerrors.Errorf("connecting to storage provider failed: %w", err)
	}
	c.connsLk.Lock()
	c.conns[deal.ProposalCid] = s
	c.connsLk.Unlock()

	err = c.statemachines.Send(deal.ProposalCid, storagemarket.ClientEventOpen)
	if err != nil {
		return cid.Undef, xerrors.Errorf("initializing state machine: %w", err)
	}

	return deal.ProposalCid, c.discovery.AddPeer(p.Data.Root, retrievalmarket.RetrievalPeer{
		Address: dealProposal.Provider,
		ID:      deal.Miner,
	})
}

func (c *Client) QueryAsk(ctx context.Context, p peer.ID, a address.Address) (*storagemarket.SignedStorageAsk, error) {
	s, err := c.net.NewAskStream(p)
	if err != nil {
		return nil, xerrors.Errorf("failed to open stream to miner: %w", err)
	}

	request := network.AskRequest{Miner: a}
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

	if out.Ask.Ask.Miner != a {
		return nil, xerrors.Errorf("got back ask for wrong miner")
	}

	if err := c.node.ValidateAskSignature(out.Ask); err != nil {
		return nil, xerrors.Errorf("ask was not properly signed")
	}

	return out.Ask, nil
}

func (c *Client) List() ([]storagemarket.ClientDeal, error) {
	var out []storagemarket.ClientDeal
	if err := c.statemachines.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetDeal(d cid.Cid) (*storagemarket.ClientDeal, error) {
	var out storagemarket.ClientDeal
	if err := c.statemachines.Get(d).Get(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Stop() {
	_ = c.statemachines.Stop(context.TODO())
}

func (c *Client) Node() storagemarket.StorageClientNode {
	return c.node
}

func (c *Client) DealStream(proposalCid cid.Cid) (network.StorageDealStream, error) {
	c.connsLk.RLock()
	s, ok := c.conns[proposalCid]
	c.connsLk.RUnlock()
	if ok {
		return s, nil
	}
	return nil, xerrors.New("no connection to provider")
}

func (c *Client) CloseStream(proposalCid cid.Cid) error {
	c.connsLk.Lock()
	defer c.connsLk.Unlock()
	s, ok := c.conns[proposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(c.conns, proposalCid)
	return err
}
