package retrievalimpl

import (
	"context"
	"errors"
	"io"
	"io/ioutil"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/pieceio/cario"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var _ requestvalidation.ValidationEnvironment = new(providerValidationEnvironment)

type providerValidationEnvironment struct {
	p *Provider
}

func (pve *providerValidationEnvironment) GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid,
	piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (retrievalmarket.Ask, error) {

	storageDeals, err := storageDealsForPiece(pieceCid != nil, payloadCid, piece, pve.p.pieceStore)
	if err != nil {
		return retrievalmarket.Ask{}, xerrors.Errorf("failed to fetch deals for payload, err=%s", err)
	}

	input := retrievalmarket.PricingInput{
		// piece from which the payload will be retrieved
		PieceCID: piece.PieceCID,

		PayloadCID: payloadCid,
		Unsealed:   isUnsealed,
		Client:     client,
	}

	return pve.p.GetDynamicAsk(ctx, input, storageDeals)
}

func (pve *providerValidationEnvironment) GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error) {
	inPieceCid := cid.Undef
	if pieceCID != nil {
		inPieceCid = *pieceCID
	}

	return getPieceInfoFromCid(context.TODO(), pve.p.node, pve.p.pieceStore, c, inPieceCid)
}

// CheckDealParams verifies the given deal params are acceptable
func (pve *providerValidationEnvironment) CheckDealParams(ask retrievalmarket.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error {
	if pricePerByte.LessThan(ask.PricePerByte) {
		return errors.New("Price per byte too low")
	}
	if paymentInterval > ask.PaymentInterval {
		return errors.New("Payment interval too large")
	}
	if paymentIntervalIncrease > ask.PaymentIntervalIncrease {
		return errors.New("Payment interval increase too large")
	}
	if !ask.UnsealPrice.Nil() && unsealPrice.LessThan(ask.UnsealPrice) {
		return errors.New("Unseal price too small")
	}
	return nil
}

// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
func (pve *providerValidationEnvironment) RunDealDecisioningLogic(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
	if pve.p.dealDecider == nil {
		return true, "", nil
	}
	return pve.p.dealDecider(ctx, state)
}

// StateMachines returns the FSM Group to begin tracking with
func (pve *providerValidationEnvironment) BeginTracking(pds retrievalmarket.ProviderDealState) error {
	err := pve.p.stateMachines.Begin(pds.Identifier(), &pds)
	if err != nil {
		return err
	}

	if pds.UnsealPrice.GreaterThan(big.Zero()) {
		return pve.p.stateMachines.Send(pds.Identifier(), retrievalmarket.ProviderEventPaymentRequested, uint64(0))
	}

	return pve.p.stateMachines.Send(pds.Identifier(), retrievalmarket.ProviderEventOpen)
}

// NextStoreID allocates a store for this deal
func (pve *providerValidationEnvironment) NextStoreID() (multistore.StoreID, error) {
	storeID := pve.p.multiStore.Next()
	_, err := pve.p.multiStore.Get(storeID)
	return storeID, err
}

type providerRevalidatorEnvironment struct {
	p *Provider
}

func (pre *providerRevalidatorEnvironment) Node() retrievalmarket.RetrievalProviderNode {
	return pre.p.node
}

func (pre *providerRevalidatorEnvironment) SendEvent(dealID retrievalmarket.ProviderDealIdentifier, evt retrievalmarket.ProviderEvent, args ...interface{}) error {
	return pre.p.stateMachines.Send(dealID, evt, args...)
}

func (pre *providerRevalidatorEnvironment) Get(dealID retrievalmarket.ProviderDealIdentifier) (retrievalmarket.ProviderDealState, error) {
	var deal retrievalmarket.ProviderDealState
	err := pre.p.stateMachines.GetSync(context.TODO(), dealID, &deal)
	return deal, err
}

var _ providerstates.ProviderDealEnvironment = new(providerDealEnvironment)

type providerDealEnvironment struct {
	p *Provider
}

// Node returns the node interface for this deal
func (pde *providerDealEnvironment) Node() retrievalmarket.RetrievalProviderNode {
	return pde.p.node
}

func (pde *providerDealEnvironment) ReadIntoBlockstore(storeID multistore.StoreID, pieceData io.ReadCloser) error {
	// Get the the destination multistore
	store, loadErr := pde.p.multiStore.Get(storeID)
	if loadErr != nil {
		return xerrors.Errorf("failed to read file into blockstore: failed to get multistore %d: %w", storeID, loadErr)
	}

	// Load the CAR into the blockstore
	_, loadErr = cario.NewCarIO().LoadCar(store.Bstore, pieceData)
	if loadErr != nil {
		// Just log the error, so we can drain and close the reader before
		// returning the error
		loadErr = xerrors.Errorf("failed to load car file into blockstore: %w", loadErr)
		log.Error(loadErr.Error())
	}

	// Attempt to drain and close the reader before returning any error
	_, drainErr := io.Copy(ioutil.Discard, pieceData)
	closeErr := pieceData.Close()

	// If there was an error loading the CAR file into the blockstore, throw that error
	if loadErr != nil {
		return loadErr
	}

	// If there was an error draining the reader, throw that error
	if drainErr != nil {
		err := xerrors.Errorf("failed to read file into blockstore: failed to drain piece reader: %w", drainErr)
		log.Error(err.Error())
		return err
	}

	// If there was an error closing the reader, throw that error
	if closeErr != nil {
		err := xerrors.Errorf("failed to read file into blockstore: failed to close reader: %w", closeErr)
		log.Error(err.Error())
		return err
	}

	return nil
}

func (pde *providerDealEnvironment) TrackTransfer(deal retrievalmarket.ProviderDealState) error {
	pde.p.revalidator.TrackChannel(deal)
	return nil
}

func (pde *providerDealEnvironment) UntrackTransfer(deal retrievalmarket.ProviderDealState) error {
	pde.p.revalidator.UntrackChannel(deal)
	return nil
}

func (pde *providerDealEnvironment) ResumeDataTransfer(ctx context.Context, chid datatransfer.ChannelID) error {
	return pde.p.dataTransfer.ResumeDataTransferChannel(ctx, chid)
}

func (pde *providerDealEnvironment) CloseDataTransfer(ctx context.Context, chid datatransfer.ChannelID) error {
	// When we close the data transfer, we also send a cancel message to the peer.
	// Make sure we don't wait too long to send the message.
	ctx, cancel := context.WithTimeout(ctx, shared.CloseDataTransferTimeout)
	defer cancel()

	err := pde.p.dataTransfer.CloseDataTransferChannel(ctx, chid)
	if shared.IsCtxDone(err) {
		log.Warnf("failed to send cancel data transfer channel %s to client within timeout %s",
			chid, shared.CloseDataTransferTimeout)
		return nil
	}
	return err
}

func (pde *providerDealEnvironment) DeleteStore(storeID multistore.StoreID) error {
	return pde.p.multiStore.Delete(storeID)
}

func pieceInUnsealedSector(ctx context.Context, n retrievalmarket.RetrievalProviderNode, pieceInfo piecestore.PieceInfo) bool {
	for _, di := range pieceInfo.Deals {
		isUnsealed, err := n.IsUnsealed(ctx, di.SectorID, di.Offset.Unpadded(), di.Length.Unpadded())
		if err != nil {
			log.Errorf("failed to find out if sector %d is unsealed, err=%s", di.SectorID, err)
			continue
		}
		if isUnsealed {
			return true
		}
	}

	return false
}

func storageDealsForPiece(clientSpecificPiece bool, payloadCID cid.Cid, pieceInfo piecestore.PieceInfo, pieceStore piecestore.PieceStore) ([]abi.DealID, error) {
	var storageDeals []abi.DealID
	var err error
	if clientSpecificPiece {
		//  If the user wants to retrieve the payload from a specific piece,
		//  we only need to inspect storage deals made for that piece to quote a price.
		for _, d := range pieceInfo.Deals {
			storageDeals = append(storageDeals, d.DealID)
		}
	} else {
		// If the user does NOT want to retrieve from a specific piece, we'll have to inspect all storage deals
		// made for that piece to quote a price.
		storageDeals, err = getAllDealsContainingPayload(pieceStore, payloadCID)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch deals for payload: %w", err)
		}
	}

	if len(storageDeals) == 0 {
		return nil, xerrors.New("no storage deals found")
	}

	return storageDeals, nil
}

func getAllDealsContainingPayload(pieceStore piecestore.PieceStore, payloadCID cid.Cid) ([]abi.DealID, error) {
	cidInfo, err := pieceStore.GetCIDInfo(payloadCID)
	if err != nil {
		return nil, xerrors.Errorf("get cid info: %w", err)
	}
	var dealsIds []abi.DealID
	var lastErr error

	for _, pieceBlockLocation := range cidInfo.PieceBlockLocations {
		pieceInfo, err := pieceStore.GetPieceInfo(pieceBlockLocation.PieceCID)
		if err != nil {
			lastErr = err
			continue
		}
		for _, d := range pieceInfo.Deals {
			dealsIds = append(dealsIds, d.DealID)
		}
	}

	if lastErr == nil && len(dealsIds) == 0 {
		return nil, xerrors.New("no deals found")
	}

	if lastErr != nil && len(dealsIds) == 0 {
		return nil, xerrors.Errorf("failed to fetch deals containing payload %s: %w", payloadCID, lastErr)
	}

	return dealsIds, nil
}

func getPieceInfoFromCid(ctx context.Context, n retrievalmarket.RetrievalProviderNode, pieceStore piecestore.PieceStore, payloadCID, pieceCID cid.Cid) (piecestore.PieceInfo, bool, error) {
	cidInfo, err := pieceStore.GetCIDInfo(payloadCID)
	if err != nil {
		return piecestore.PieceInfoUndefined, false, xerrors.Errorf("get cid info: %w", err)
	}
	var lastErr error
	var sealedPieceInfo *piecestore.PieceInfo

	for _, pieceBlockLocation := range cidInfo.PieceBlockLocations {
		pieceInfo, err := pieceStore.GetPieceInfo(pieceBlockLocation.PieceCID)
		if err != nil {
			lastErr = err
			continue
		}

		// if client wants to retrieve the payload from a specific piece, just return that piece.
		if pieceCID.Defined() && pieceInfo.PieceCID.Equals(pieceCID) {
			return pieceInfo, pieceInUnsealedSector(ctx, n, pieceInfo), nil
		}

		// if client dosen't have a preference for a particular piece, prefer a piece
		// for which an unsealed sector exists.
		if pieceCID.Equals(cid.Undef) {
			if pieceInUnsealedSector(ctx, n, pieceInfo) {
				return pieceInfo, true, nil
			}

			if sealedPieceInfo == nil {
				sealedPieceInfo = &pieceInfo
			}
		}

	}

	if sealedPieceInfo != nil {
		return *sealedPieceInfo, false, nil
	}

	if lastErr == nil {
		lastErr = xerrors.Errorf("unknown pieceCID %s", pieceCID.String())
	}

	return piecestore.PieceInfoUndefined, false, xerrors.Errorf("could not locate piece: %w", lastErr)
}

var _ dtutils.StoreGetter = &providerStoreGetter{}

type providerStoreGetter struct {
	p *Provider
}

func (psg *providerStoreGetter) Get(otherPeer peer.ID, dealID retrievalmarket.DealID) (*multistore.Store, error) {
	var deal retrievalmarket.ProviderDealState
	provDealID := retrievalmarket.ProviderDealIdentifier{Receiver: otherPeer, DealID: dealID}
	err := psg.p.stateMachines.Get(provDealID).Get(&deal)
	if err != nil {
		return nil, err
	}
	return psg.p.multiStore.Get(deal.StoreID)
}
