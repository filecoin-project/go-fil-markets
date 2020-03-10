package storageimpl

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/connmanager"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var ProviderDsPrefix = "/deals/provider"
var _ storagemarket.StorageProvider = &Provider{}

type Provider struct {
	net network.StorageMarketNetwork

	proofType abi.RegisteredProof

	spn          storagemarket.StorageProviderNode
	fs           filestore.FileStore
	pio          pieceio.PieceIOWithStore
	pieceStore   piecestore.PieceStore
	conns        *connmanager.ConnManager
	storedAsk    *storedask.StoredAsk
	actor        address.Address
	dataTransfer datatransfer.Manager

	deals fsm.Group
}

func NewProvider(net network.StorageMarketNetwork, ds datastore.Batching, bs blockstore.Blockstore, fs filestore.FileStore, pieceStore piecestore.PieceStore, dataTransfer datatransfer.Manager, spn storagemarket.StorageProviderNode, minerAddress address.Address, rt abi.RegisteredProof) (storagemarket.StorageProvider, error) {
	carIO := cario.NewCarIO()
	pio := pieceio.NewPieceIOWithStore(carIO, fs, bs)

	storedAsk, err := storedask.NewStoredAsk(ds, spn, minerAddress)
	if err != nil {
		return nil, err
	}

	h := &Provider{
		net:          net,
		proofType:    rt,
		spn:          spn,
		fs:           fs,
		pio:          pio,
		pieceStore:   pieceStore,
		conns:        connmanager.NewConnManager(),
		storedAsk:    storedAsk,
		actor:        minerAddress,
		dataTransfer: dataTransfer,
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

	// register a data transfer event handler -- this will move deals from
	// accepted to staged
	dataTransfer.SubscribeToEvents(providerutils.DataTransferSubscriber(deals))

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
	err = p.conns.AddStream(proposalNd.Cid(), s)
	if err != nil {
		return err
	}
	return p.deals.Send(proposalNd.Cid(), storagemarket.ProviderEventOpen)
}

func (p *Provider) Stop() error {
	err := p.deals.Stop(context.TODO())
	if err != nil {
		return err
	}
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
	ask := p.storedAsk.GetAsk(addr)
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
	return p.spn.GetBalance(ctx, p.actor)
}

func (p *Provider) ListIncompleteDeals() ([]storagemarket.MinerDeal, error) {
	var out []storagemarket.MinerDeal
	if err := p.deals.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) AddAsk(price abi.TokenAmount, duration abi.ChainEpoch) error {
	return p.storedAsk.AddAsk(price, duration)
}

func (p *Provider) HandleAskStream(s network.StorageAskStream) {
	defer s.Close()
	ar, err := s.ReadAskRequest()
	if err != nil {
		log.Errorf("failed to read AskRequest from incoming stream: %s", err)
		return
	}

	resp := network.AskResponse{
		Ask: p.storedAsk.GetAsk(ar.Miner),
	}

	if err := s.WriteAskResponse(resp); err != nil {
		log.Errorf("failed to write ask response: %s", err)
		return
	}
}

type providerDealEnvironment struct {
	p *Provider
}

func (p *providerDealEnvironment) Address() address.Address {
	return p.p.actor
}

func (p *providerDealEnvironment) Node() storagemarket.StorageProviderNode {
	return p.p.spn
}

func (p *providerDealEnvironment) Ask() storagemarket.StorageAsk {
	sask := p.p.storedAsk.GetAsk(p.p.actor)
	if sask == nil {
		return storagemarket.StorageAskUndefined
	}
	return *sask.Ask
}

func (p *providerDealEnvironment) StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	_, err := p.p.dataTransfer.OpenPullDataChannel(ctx, to, voucher, baseCid, selector)
	return err
}

func (p *providerDealEnvironment) GeneratePieceCommitmentToFile(payloadCid cid.Cid, selector ipld.Node) (cid.Cid, filestore.Path, error) {
	pieceCid, path, _, err := p.p.pio.GeneratePieceCommitmentToFile(p.p.proofType, payloadCid, selector)
	return pieceCid, path, err
}

func (p *providerDealEnvironment) OpenFile(path filestore.Path) (filestore.File, error) {
	return p.p.fs.Open(path)
}

func (p *providerDealEnvironment) DeleteFile(path filestore.Path) error {
	return p.p.fs.Delete(path)
}

func (p *providerDealEnvironment) AddDealForPiece(pieceCID cid.Cid, dealInfo piecestore.DealInfo) error {
	return p.p.pieceStore.AddDealForPiece(pieceCID, dealInfo)
}

func (p *providerDealEnvironment) AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]piecestore.BlockLocation) error {
	return p.p.pieceStore.AddPieceBlockLocations(pieceCID, blockLocations)
}

func (p *providerDealEnvironment) SendSignedResponse(ctx context.Context, resp *network.Response) error {
	s, err := p.p.conns.DealStream(resp.Proposal)
	if err != nil {
		return xerrors.Errorf("couldn't send response: %w", err)
	}

	sig, err := providerutils.SignMinerData(resp, ctx, p.p.actor, p.Node().GetMinerWorker, p.Node().SignBytes)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := network.SignedResponse{
		Response:  *resp,
		Signature: sig,
	}

	err = s.WriteDealResponse(signedResponse)
	if err != nil {
		// Assume client disconnected
		_ = p.p.conns.Disconnect(resp.Proposal)
	}
	return err
}

func (p *providerDealEnvironment) Disconnect(proposalCid cid.Cid) error {
	return p.p.conns.Disconnect(proposalCid)
}
