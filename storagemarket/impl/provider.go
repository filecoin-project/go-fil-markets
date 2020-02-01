package storageimpl

import (
	"context"
	"errors"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-statestore"
)

var ProviderDsPrefix = "/deals/provider"

//go:generate cbor-gen-for MinerDeal

type MinerDeal struct {
	storagemarket.MinerDeal
	s inet.Stream
}

type Provider struct {
	pricePerByteBlock tokenamount.TokenAmount // how much we want for storing one byte for one block
	minPieceSize      uint64

	ask   *types.SignedStorageAsk
	askLk sync.Mutex

	spn storagemarket.StorageProviderNode

	fs         filestore.FileStore
	pio        pieceio.PieceIOWithStore
	pieceStore piecestore.PieceStore

	// dataTransfer is the manager of data transfers used by this storage provider
	dataTransfer datatransfer.Manager

	deals *statestore.StateStore
	ds    datastore.Batching

	conns map[cid.Cid]inet.Stream

	actor address.Address

	incoming chan MinerDeal
	updated  chan minerDealUpdate
	stop     chan struct{}
	stopped  chan struct{}
}

type minerDealUpdate struct {
	newState storagemarket.StorageDealStatus
	id       cid.Cid
	err      error
	mut      func(*MinerDeal)
}

var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

func NewProvider(ds datastore.Batching, bs blockstore.Blockstore, fs filestore.FileStore, pieceStore piecestore.PieceStore, dataTransfer datatransfer.Manager, spn storagemarket.StorageProviderNode, minerAddress address.Address) (storagemarket.StorageProvider, error) {
	carIO := cario.NewCarIO()
	pio := pieceio.NewPieceIOWithStore(carIO, fs, bs)

	h := &Provider{
		fs:           fs,
		pio:          pio,
		pieceStore:   pieceStore,
		dataTransfer: dataTransfer,
		spn:          spn,

		pricePerByteBlock: tokenamount.FromInt(3), // TODO: allow setting
		minPieceSize:      256,                    // TODO: allow setting (BUT KEEP MIN 256! (because of how we fill sectors up))

		conns: map[cid.Cid]inet.Stream{},

		incoming: make(chan MinerDeal),
		updated:  make(chan minerDealUpdate),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),

		actor: minerAddress,

		deals: statestore.New(namespace.Wrap(ds, datastore.NewKey(ProviderDsPrefix))),
		ds:    ds,
	}

	if err := h.tryLoadAsk(); err != nil {
		return nil, err
	}

	if h.ask == nil {
		// TODO: we should be fine with this state, and just say it means 'not actively accepting deals'
		// for now... lets just set a price
		if err := h.SetPrice(tokenamount.FromInt(500_000_000), 1000000); err != nil {
			return nil, xerrors.Errorf("failed setting a default price: %w", err)
		}
	}

	// register a data transfer event handler -- this will move deals from
	// accepted to staged
	h.dataTransfer.SubscribeToEvents(h.onDataTransferEvent)

	return h, nil
}

func (p *Provider) Run(ctx context.Context, host host.Host) {
	// TODO: restore state

	host.SetStreamHandler(storagemarket.DealProtocolID, p.HandleStream)
	host.SetStreamHandler(storagemarket.AskProtocolID, p.HandleAskStream)

	go func() {
		defer log.Warn("quitting deal provider loop")
		defer close(p.stopped)

		for {
			select {
			case deal := <-p.incoming:
				p.onIncoming(deal)
			case update := <-p.updated:
				p.onUpdated(ctx, update)
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *Provider) onIncoming(deal MinerDeal) {
	log.Info("incoming deal")

	p.conns[deal.ProposalCid] = deal.s

	if err := p.deals.Begin(deal.ProposalCid, &deal); err != nil {
		// This can happen when client re-sends proposal
		p.failDeal(context.TODO(), deal.ProposalCid, err)
		log.Errorf("deal tracking failed: %s", err)
		return
	}

	go func() {
		p.updated <- minerDealUpdate{
			newState: storagemarket.StorageDealValidating,
			id:       deal.ProposalCid,
			err:      nil,
		}
	}()
}

func (p *Provider) onUpdated(ctx context.Context, update minerDealUpdate) {
	log.Infof("Deal %s updated state to %s", update.id, storagemarket.DealStates[update.newState])
	if update.err != nil {
		log.Errorf("deal %s (newSt: %d) failed: %+v", update.id, update.newState, update.err)
		p.failDeal(ctx, update.id, update.err)
		return
	}
	var deal MinerDeal
	err := p.deals.Get(update.id).Mutate(func(d *MinerDeal) error {
		d.State = update.newState
		if update.mut != nil {
			update.mut(d)
		}
		deal = *d
		return nil
	})
	if err != nil {
		p.failDeal(ctx, update.id, err)
		return
	}

	switch update.newState {
	case storagemarket.StorageDealValidating:
		p.handle(ctx, deal, p.validating, storagemarket.StorageDealTransferring)
	case storagemarket.StorageDealTransferring:
		p.handle(ctx, deal, p.transferring, storagemarket.StorageDealNoUpdate)
	case storagemarket.StorageDealVerifyData:
		p.handle(ctx, deal, p.verifydata, storagemarket.StorageDealPublishing)
	case storagemarket.StorageDealPublishing:
		p.handle(ctx, deal, p.publishing, storagemarket.StorageDealStaged)
	case storagemarket.StorageDealStaged:
		p.handle(ctx, deal, p.staged, storagemarket.StorageDealSealing)
	case storagemarket.StorageDealSealing:
		p.handle(ctx, deal, p.sealing, storagemarket.StorageDealNoUpdate)
	case storagemarket.StorageDealActive:
		p.handle(ctx, deal, p.complete, storagemarket.StorageDealNoUpdate)
	}
}

// onDataTransferEvent is the function called when an event occurs in a data
// transfer -- it reads the voucher to verify this even occurred in a storage
// market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func (p *Provider) onDataTransferEvent(event datatransfer.Event, channelState datatransfer.ChannelState) {
	voucher, ok := channelState.Voucher().(*StorageDataTransferVoucher)
	// if this event is for a transfer not related to storage, ignore
	if !ok {
		return
	}

	// data transfer events for opening and progress do not affect deal state
	var next storagemarket.StorageDealStatus
	var err error
	var mut func(*MinerDeal)
	switch event.Code {
	case datatransfer.Complete:
		next = storagemarket.StorageDealVerifyData
	case datatransfer.Error:
		next = storagemarket.StorageDealFailing
		err = ErrDataTransferFailed
	default:
		// the only events we care about are complete and error
		return
	}

	select {
	case p.updated <- minerDealUpdate{
		newState: next,
		id:       voucher.Proposal,
		err:      err,
		mut:      mut,
	}:
	case <-p.stop:
	}
}

func (p *Provider) newDeal(s inet.Stream, proposal Proposal) (MinerDeal, error) {
	proposalNd, err := cborutil.AsIpld(proposal.DealProposal)
	if err != nil {
		return MinerDeal{}, err
	}

	return MinerDeal{
		MinerDeal: storagemarket.MinerDeal{
			Client:      s.Conn().RemotePeer(),
			Proposal:    *proposal.DealProposal,
			ProposalCid: proposalNd.Cid(),
			State:       storagemarket.StorageDealUnknown,

			Ref: proposal.Piece,
		},
		s: s,
	}, nil
}

func (p *Provider) HandleStream(s inet.Stream) {
	log.Info("Handling storage deal proposal!")

	proposal, err := p.readProposal(s)
	if err != nil {
		log.Error(err)
		s.Close()
		return
	}

	deal, err := p.newDeal(s, proposal)
	if err != nil {
		log.Errorf("%+v", err)
		s.Close()
		return
	}

	p.incoming <- deal
}

func (p *Provider) Stop() {
	close(p.stop)
	<-p.stopped
}
