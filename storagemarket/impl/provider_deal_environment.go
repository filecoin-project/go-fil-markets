package storageimpl

import (
	"context"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

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
	p.p.askLk.RLock()
	defer p.p.askLk.RUnlock()
	return *p.p.ask.Ask
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
	p.p.connsLk.RLock()
	s, ok := p.p.conns[resp.Proposal]
	p.p.connsLk.RUnlock()
	if !ok {
		return xerrors.New("couldn't send response: not connected")
	}

	msg, err := cborutil.Dump(resp)
	if err != nil {
		return xerrors.Errorf("serializing response: %w", err)
	}

	worker, err := p.Node().GetMinerWorker(ctx, p.Address())
	if err != nil {
		return err
	}

	sig, err := p.Node().SignBytes(ctx, worker, msg)
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
		s.Close()
		delete(p.p.conns, resp.Proposal)
	}
	return err
}

func (p *providerDealEnvironment) Disconnect(proposalCid cid.Cid) error {
	p.p.connsLk.Lock()
	defer p.p.connsLk.Unlock()
	s, ok := p.p.conns[proposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(p.p.conns, proposalCid)
	return err
}
