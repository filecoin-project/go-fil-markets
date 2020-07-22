package retrievalimpl

import (
	"context"
	"errors"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
)

var _ requestvalidation.ValidationEnvironment = new(providerValidationEnvironment)

type providerValidationEnvironment struct {
	p *Provider
}

func (pve *providerValidationEnvironment) GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, error) {
	inPieceCid := cid.Undef
	if pieceCID != nil {
		inPieceCid = *pieceCID
	}
	return getPieceInfoFromCid(pve.p.pieceStore, c, inPieceCid)
}

// CheckDealParams verifies the given deal params are acceptable
func (pve *providerValidationEnvironment) CheckDealParams(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error {
	if pricePerByte.LessThan(pve.p.pricePerByte) {
		return errors.New("Price per byte too low")
	}
	if paymentInterval > pve.p.paymentInterval {
		return errors.New("Payment interval too large")
	}
	if paymentIntervalIncrease > pve.p.paymentIntervalIncrease {
		return errors.New("Payment interval increase too large")
	}
	if !pve.p.unsealPrice.Nil() && unsealPrice.LessThan(pve.p.unsealPrice) {
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

func (pde *providerDealEnvironment) ReadIntoBlockstore(pieceData io.Reader) error {
	_, err := cario.NewCarIO().LoadCar(pde.p.bs, pieceData)
	return err
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
	return pde.p.dataTransfer.CloseDataTransferChannel(ctx, chid)
}

func getPieceInfoFromCid(pieceStore piecestore.PieceStore, payloadCID, pieceCID cid.Cid) (piecestore.PieceInfo, error) {
	cidInfo, err := pieceStore.GetCIDInfo(payloadCID)
	if err != nil {
		return piecestore.PieceInfoUndefined, xerrors.Errorf("get cid info: %w", err)
	}
	var lastErr error
	for _, pieceBlockLocation := range cidInfo.PieceBlockLocations {
		pieceInfo, err := pieceStore.GetPieceInfo(pieceBlockLocation.PieceCID)
		if err == nil {
			if pieceCID.Equals(cid.Undef) || pieceInfo.PieceCID.Equals(pieceCID) {
				return pieceInfo, nil
			}
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = xerrors.Errorf("unknown pieceCID %s", pieceCID.String())
	}
	return piecestore.PieceInfoUndefined, xerrors.Errorf("could not locate piece: %w", lastErr)
}
