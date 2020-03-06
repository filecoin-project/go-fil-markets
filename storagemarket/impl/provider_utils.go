package storageimpl

import (
	"bytes"
	"context"
	"runtime"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

func (p *Provider) failDeal(ctx context.Context, id cid.Cid, cerr error) {
	if err := p.deals.Get(id).End(); err != nil {
		log.Warnf("deals.End: %s", err)
	}

	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	log.Warnf("deal %s failed: %s", id, cerr)

	err := p.sendSignedResponse(ctx, &network.Response{
		State:    storagemarket.StorageDealFailing,
		Message:  cerr.Error(),
		Proposal: id,
	})

	s, ok := p.conns[id]
	if ok {
		_ = s.Close()
		delete(p.conns, id)
	}

	if err != nil {
		log.Warnf("notifying client about deal failure: %s", err)
	}
}

func (p *Provider) verifyProposal(sdp *market.ClientDealProposal) error {
	var buf bytes.Buffer
	if err := sdp.Proposal.MarshalCBOR(&buf); err != nil {
		return err
	}
	verified := p.spn.VerifySignature(sdp.ClientSignature, sdp.Proposal.Client, buf.Bytes())
	if !verified {
		return xerrors.New("could not verify signature")
	}
	return nil
}
func (p *Provider) readProposal(s network.StorageDealStream) (proposal network.Proposal, err error) {
	proposal, err = s.ReadDealProposal()
	if err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return proposal, err
	}

	if err := p.verifyProposal(proposal.DealProposal); err != nil {
		return proposal, xerrors.Errorf("verifying StorageDealProposal: %w", err)
	}

	if proposal.DealProposal.Proposal.Provider != p.actor {
		log.Errorf("proposal with wrong ProviderAddress: %s", proposal.DealProposal.Proposal.Provider)
		return proposal, err
	}

	return
}

func (p *Provider) sendSignedResponse(ctx context.Context, resp *network.Response) error {
	s, ok := p.conns[resp.Proposal]
	if !ok {
		return xerrors.New("couldn't send response: not connected")
	}

	msg, err := cborutil.Dump(resp)
	if err != nil {
		return xerrors.Errorf("serializing response: %w", err)
	}

	worker, err := p.spn.GetMinerWorker(ctx, p.actor)
	if err != nil {
		return err
	}

	sig, err := p.spn.SignBytes(ctx, worker, msg)
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
		delete(p.conns, resp.Proposal)
	}
	return err
}

func (p *Provider) disconnect(deal MinerDeal) error {
	s, ok := p.conns[deal.ProposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(p.conns, deal.ProposalCid)
	return err
}
