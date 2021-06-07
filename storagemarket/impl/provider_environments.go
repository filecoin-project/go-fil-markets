package storageimpl

import (
	"context"
	"errors"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// -------
// providerDealEnvironment
// -------

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
	sask := p.p.storedAsk.GetAsk()
	if sask == nil {
		return storagemarket.StorageAskUndefined
	}
	return *sask.Ask
}

func (p *providerDealEnvironment) DeleteStore(storeID multistore.StoreID) error {
	return p.p.multiStore.Delete(storeID)
}

func (p *providerDealEnvironment) GeneratePieceCommitment(storeID *multistore.StoreID, payloadCid cid.Cid, selector ipld.Node, pieceSize abi.PaddedPieceSize) (cid.Cid, filestore.Path, error) {
	proofType, err := p.p.spn.GetProofType(context.TODO(), p.p.actor, nil)
	if err != nil {
		return cid.Undef, "", err
	}

	var pieceCid cid.Cid
	var path filestore.Path
	var psize abi.UnpaddedPieceSize

	if p.p.universalRetrievalEnabled {
		pieceCid, psize, path, err = providerutils.GeneratePieceCommitmentWithMetadata(p.p.fs, p.p.pio.GeneratePieceCommitment, proofType, payloadCid, selector, storeID)
	} else {
		pieceCid, psize, err = p.p.pio.GeneratePieceCommitment(proofType, payloadCid, selector, storeID)
	}
	if err != nil {
		return cid.Undef, "", err
	}

	if psize.Padded() < pieceSize {
		// need to pad up!
		rawPaddedCommp, err := commp.PadCommP(
			// we know how long a pieceCid "hash" is, just blindly extract the trailing 32 bytes
			pieceCid.Hash()[len(pieceCid.Hash())-32:],
			uint64(psize.Padded()),
			uint64(pieceSize),
		)
		if err != nil {
			return cid.Undef, "", err
		}
		pieceCid, _ = commcid.DataCommitmentV1ToCID(rawPaddedCommp)
	}

	return pieceCid, path, nil
}

func (p *providerDealEnvironment) GeneratePieceReader(storeID *multistore.StoreID, payloadCid cid.Cid, selector ipld.Node) (io.ReadCloser, uint64, error, <-chan error) {
	return p.p.pio.GeneratePieceReader(payloadCid, selector, storeID)
}

func (p *providerDealEnvironment) FileStore() filestore.FileStore {
	return p.p.fs
}

func (p *providerDealEnvironment) PieceStore() piecestore.PieceStore {
	return p.p.pieceStore
}

func (p *providerDealEnvironment) SendSignedResponse(ctx context.Context, resp *network.Response) error {
	s, err := p.p.conns.DealStream(resp.Proposal)
	if err != nil {
		return xerrors.Errorf("couldn't send response: %w", err)
	}

	sig, err := p.p.sign(ctx, resp)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := network.SignedResponse{
		Response:  *resp,
		Signature: sig,
	}

	err = s.WriteDealResponse(signedResponse, p.p.sign)
	if err != nil {
		// Assume client disconnected
		_ = p.p.conns.Disconnect(resp.Proposal)
	}
	return err
}

func (p *providerDealEnvironment) Disconnect(proposalCid cid.Cid) error {
	return p.p.conns.Disconnect(proposalCid)
}

func (p *providerDealEnvironment) RunCustomDecisionLogic(ctx context.Context, deal storagemarket.MinerDeal) (bool, string, error) {
	if p.p.customDealDeciderFunc == nil {
		return true, "", nil
	}
	return p.p.customDealDeciderFunc(ctx, deal)
}

func (p *providerDealEnvironment) TagPeer(id peer.ID, s string) {
	p.p.net.TagPeer(id, s)
}

func (p *providerDealEnvironment) UntagPeer(id peer.ID, s string) {
	p.p.net.UntagPeer(id, s)
}

var _ providerstates.ProviderDealEnvironment = &providerDealEnvironment{}

type providerStoreGetter struct {
	p *Provider
}

func (psg *providerStoreGetter) Get(proposalCid cid.Cid) (*multistore.Store, error) {
	// Wait for the provider to be ready
	err := awaitProviderReady(psg.p)
	if err != nil {
		return nil, err
	}

	var deal storagemarket.MinerDeal
	err = psg.p.deals.Get(proposalCid).Get(&deal)
	if err != nil {
		return nil, err
	}
	if deal.StoreID == nil {
		return nil, errors.New("No store for this deal")
	}
	return psg.p.multiStore.Get(*deal.StoreID)
}

type providerPushDeals struct {
	p *Provider
}

func (ppd *providerPushDeals) Get(proposalCid cid.Cid) (storagemarket.MinerDeal, error) {
	// Wait for the provider to be ready
	var deal storagemarket.MinerDeal
	err := awaitProviderReady(ppd.p)
	if err != nil {
		return deal, err
	}

	err = ppd.p.deals.GetSync(context.TODO(), proposalCid, &deal)
	return deal, err
}

// awaitProviderReady waits for the provider to startup
func awaitProviderReady(p *Provider) error {
	err := p.AwaitReady()
	if err != nil {
		return xerrors.Errorf("could not get deal with proposal CID %s: error waiting for provider startup: %w")
	}

	return nil
}
