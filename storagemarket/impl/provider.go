package storageimpl

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var ProviderDsPrefix = "/deals/provider"

type Provider struct {
	net network.StorageMarketNetwork

	minPieceSize abi.PaddedPieceSize
	proofType    abi.RegisteredProof

	askLk sync.RWMutex
	ask   *storagemarket.SignedStorageAsk

	spn storagemarket.StorageProviderNode

	fs         filestore.FileStore
	pio        pieceio.PieceIOWithStore
	pieceStore piecestore.PieceStore

	// dataTransfer is the manager of data transfers used by this storage provider
	dataTransfer datatransfer.Manager

	deals fsm.Group
	ds    datastore.Batching

	connsLk sync.RWMutex
	conns   map[cid.Cid]network.StorageDealStream

	actor address.Address
}

var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

func NewProvider(net network.StorageMarketNetwork, ds datastore.Batching, bs blockstore.Blockstore, fs filestore.FileStore, pieceStore piecestore.PieceStore, dataTransfer datatransfer.Manager, spn storagemarket.StorageProviderNode, minerAddress address.Address, rt abi.RegisteredProof) (storagemarket.StorageProvider, error) {
	carIO := cario.NewCarIO()
	pio := pieceio.NewPieceIOWithStore(carIO, fs, bs)

	h := &Provider{
		net:          net,
		fs:           fs,
		pio:          pio,
		pieceStore:   pieceStore,
		dataTransfer: dataTransfer,
		spn:          spn,

		minPieceSize: 256, // TODO: allow setting (BUT KEEP MIN 256! (because of how we fill sectors up))
		proofType:    rt,

		conns: map[cid.Cid]network.StorageDealStream{},

		actor: minerAddress,

		ds: ds,
	}

	deals, err := fsm.New(namespace.Wrap(ds, datastore.NewKey(ProviderDsPrefix)), fsm.Parameters{
		Environment:     &providerDealEnvironment{h},
		StateType:       storagemarket.MinerDeal{},
		StateKeyField:   "State",
		Events:          providerstates.ProviderEvents,
		StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
	})
	if err != nil {
		return nil, err
	}

	h.deals = deals

	if err := h.tryLoadAsk(); err != nil {
		return nil, err
	}

	if h.ask == nil {
		// TODO: we should be fine with this state, and just say it means 'not actively accepting deals'
		// for now... lets just set a price
		if err := h.AddAsk(abi.NewTokenAmount(500_000_000), 1000000); err != nil {
			return nil, xerrors.Errorf("failed setting a default price: %w", err)
		}
	}

	// register a data transfer event handler -- this will move deals from
	// accepted to staged
	h.dataTransfer.SubscribeToEvents(h.onDataTransferEvent)

	return h, nil
}

func (p *Provider) Start(ctx context.Context) error {
	// TODO: restore state
	err := p.net.SetDelegate(p)
	if err != nil {
		return err
	}
	return nil
}

// onDataTransferEvent is the function called when an event occurs in a data
// transfer -- it reads the voucher to verify this even occurred in a storage
// market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func (p *Provider) onDataTransferEvent(event datatransfer.Event, channelState datatransfer.ChannelState) {
	voucher, ok := channelState.Voucher().(*requestvalidation.StorageDataTransferVoucher)
	// if this event is for a transfer not related to storage, ignore
	if !ok {
		return
	}

	// data transfer events for opening and progress do not affect deal state
	switch event.Code {
	case datatransfer.Complete:
		err := p.deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferCompleted)
		if err != nil {
			log.Errorf("processing dt event: %w", err)
		}
	case datatransfer.Error:
		err := p.deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferFailed, ErrDataTransferFailed)
		if err != nil {
			log.Errorf("processing dt event: %w", err)
		}
	default:
	}
}

func (p *Provider) HandleDealStream(s network.StorageDealStream) {
	log.Info("Handling storage deal proposal!")

	err := p.receiveDeal(s)
	if err != nil {
		log.Errorf("%+v", err)
		s.Close()
		return
	}
}

func (p *Provider) receiveDeal(s network.StorageDealStream) error {
	proposal, err := s.ReadDealProposal()
	if err != nil {
		return xerrors.Errorf("failed to read proposal message: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(proposal.DealProposal)
	if err != nil {
		return err
	}

	deal := &storagemarket.MinerDeal{
		Client:             s.RemotePeer(),
		ClientDealProposal: *proposal.DealProposal,
		ProposalCid:        proposalNd.Cid(),
		State:              storagemarket.StorageDealUnknown,
		Ref:                proposal.Piece,
	}

	err = p.deals.Begin(proposalNd.Cid(), deal)
	if err != nil {
		return err
	}
	p.connsLk.Lock()
	p.conns[proposalNd.Cid()] = s
	p.connsLk.Unlock()

	return p.deals.Send(proposalNd.Cid(), storagemarket.ProviderEventOpen)
}

func (p *Provider) Stop() error {
	p.deals.Stop(context.TODO())
	return p.net.StopHandlingRequests()
}

func (p *Provider) ImportDataForDeal(ctx context.Context, propCid cid.Cid, data io.Reader) error {
	// TODO: be able to check if we have enough disk space
	var d storagemarket.MinerDeal
	if err := p.deals.Get(propCid).Get(&d); err != nil {
		return xerrors.Errorf("failed getting deal %s: %w", propCid, err)
	}

	tempfi, err := p.fs.CreateTemp()
	if err != nil {
		return xerrors.Errorf("failed to create temp file for data import: %w", err)
	}

	n, err := io.Copy(tempfi, data)
	if err != nil {
		return xerrors.Errorf("importing deal data failed: %w", err)
	}
	_ = n // TODO: verify n?

	pieceSize := uint64(tempfi.Size())

	_, err = tempfi.Seek(0, io.SeekStart)
	if err != nil {
		return xerrors.Errorf("failed to seek through temp imported file: %w", err)
	}

	pieceCid, _, err := pieceio.GeneratePieceCommitment(p.proofType, tempfi, pieceSize)
	if err != nil {
		return xerrors.Errorf("failed to generate commP")
	}

	// Verify CommP matches
	if !pieceCid.Equals(d.Proposal.PieceCID) {
		return xerrors.Errorf("given data does not match expected commP (got: %x, expected %x)", pieceCid, d.Proposal.PieceCID)
	}

	return p.deals.Send(propCid, storagemarket.ProviderEventVerifiedData, tempfi.Path())
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

	if err := p.deals.List(&out); err != nil {
		return nil, err
	}

	return out, nil
}

var _ storagemarket.StorageProvider = &Provider{}
