package storageimpl

import (
	"context"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/writer"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// -------
// providerDealEnvironment
// -------

type providerDealEnvironment struct {
	p *Provider
}

func (p *providerDealEnvironment) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool) error {
	return shared.RegisterShardSync(ctx, p.p.dagStore, pieceCid, carPath, eagerInit)
}

func (p *providerDealEnvironment) CARv2Reader(carV2FilePath string) (*carv2.Reader, error) {
	return carv2.OpenReader(carV2FilePath)
}

func (p *providerDealEnvironment) FinalizeReadWriteBlockstore(proposalCid cid.Cid, carPath string, rootCid cid.Cid) error {
	bs, err := p.p.readWriteBlockStores.GetOrCreate(proposalCid.String(), carPath, rootCid)
	if err != nil {
		return xerrors.Errorf("failed to get read-write blockstore: %w", err)
	}

	if err := bs.Finalize(); err != nil {
		return xerrors.Errorf("failed to finalize read-write blockstore: %w", err)
	}

	return nil
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

func (p *providerDealEnvironment) CleanReadWriteBlockstore(proposalCid cid.Cid, carV2FilePath string) error {
	// close the backing CARv2 file and stop tracking the read-write blockstore for the deal with the given proposalCid.
	if err := p.p.readWriteBlockStores.CleanBlockstore(proposalCid.String()); err != nil {
		log.Warnf("failed to clean read write blockstore, proposalCid=%s, carV2FilePath=%s: %s", proposalCid, carV2FilePath, err)
	}

	// clean up the backing CARv2 file as it was a temporary file we created for this Storage deal and the deal dag has
	// now either been sealed into a Sector or the storage deal has failed.
	return os.Remove(carV2FilePath)
}

// GeneratePieceCommitment generates the pieceCid for the CARv1 deal payload in the CARv2 file that already exists at the given path.
func (p *providerDealEnvironment) GeneratePieceCommitment(proposalCid cid.Cid, carV2FilePath string) (c cid.Cid, path filestore.Path, finalErr error) {
	rd, err := carv2.OpenReader(carV2FilePath)
	if err != nil {
		return cid.Undef, "", xerrors.Errorf("failed to get CARv2 reader, proposalCid=%s, carV2FilePath=%s: %w", proposalCid, carV2FilePath, err)
	}

	defer func() {
		if err := rd.Close(); err != nil {
			log.Errorf("failed to close CARv2 reader, carV2FilePath=%s, err=%s", carV2FilePath, err)

			if finalErr == nil {
				c = cid.Undef
				path = ""
				finalErr = xerrors.Errorf("failed to close CARv2 reader, proposalCid=%s, carV2FilePath=%s, err=%s",
					proposalCid, carV2FilePath, err)
				return
			}
		}
	}()

	// TODO Get this work later = punt on it for now as this is anyways NOT enabled.
	/*if p.p.universalRetrievalEnabled {
		//return providerutils.GeneratePieceCommitmentWithMetadata(p.p.fs, p.p.pio.GeneratePieceCommitment, proofType, payloadCid, selector, storeID)
	}*/

	// dump the CARv1 payload of the CARv2 file to the Commp Writer and get back the CommP.
	w := &writer.Writer{}
	written, err := io.Copy(w, rd.DataReader())
	if err != nil {
		return cid.Undef, "", xerrors.Errorf("failed to write to CommP writer: %w", err)
	}
	if written != int64(rd.Header.DataSize) {
		return cid.Undef, "", xerrors.Errorf("number of bytes written to CommP writer %d not equal to the CARv1 payload size %d", written, rd.Header.DataSize)
	}

	cidAndSize, err := w.Sum()
	if err != nil {
		return cid.Undef, "", xerrors.Errorf("failed to get CommP: %w", err)
	}

	return cidAndSize.PieceCID, filestore.Path(""), err
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

func (psg *providerStoreGetter) Get(proposalCid cid.Cid) (bstore.Blockstore, error) {
	// Wait for the provider to be ready
	err := awaitProviderReady(psg.p)
	if err != nil {
		return nil, err
	}

	var deal storagemarket.MinerDeal
	err = psg.p.deals.Get(proposalCid).Get(&deal)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deal state: %w", err)
	}

	return psg.p.readWriteBlockStores.GetOrCreate(proposalCid.String(), deal.CARv2FilePath, deal.Ref.Root)
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
