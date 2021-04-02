package requestvalidation

import (
	"bytes"
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	peer "github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var allSelectorBytes []byte

func init() {
	buf := new(bytes.Buffer)
	_ = dagcbor.Encoder(shared.AllSelector(), buf)
	allSelectorBytes = buf.Bytes()
}

// ValidationEnvironment contains the dependencies needed to validate deals
type ValidationEnvironment interface {
	GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, error)
	// CheckDealParams verifies the given deal params are acceptable
	CheckDealParams(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error
	// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
	RunDealDecisioningLogic(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error)
	// StateMachines returns the FSM Group to begin tracking with
	BeginTracking(pds retrievalmarket.ProviderDealState) error
	// NextStoreID allocates a store for this deal
	NextStoreID() (multistore.StoreID, error)
}

// ProviderRequestValidator validates incoming requests for the Retrieval Provider
type ProviderRequestValidator struct {
	env ValidationEnvironment
}

// NewProviderRequestValidator returns a new instance of the ProviderRequestValidator
func NewProviderRequestValidator(env ValidationEnvironment) *ProviderRequestValidator {
	return &ProviderRequestValidator{env}
}

// ValidatePush validates a push request received from the peer that will send data
func (rv *ProviderRequestValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, errors.New("No pushes accepted")
}

// ValidatePull validates a pull request received from the peer that will receive data
func (rv *ProviderRequestValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	proposal, ok := voucher.(*retrievalmarket.DealProposal)
	var legacyProtocol bool
	if !ok {
		legacyProposal, ok := voucher.(*migrations.DealProposal0)
		if !ok {
			return nil, errors.New("wrong voucher type")
		}
		newProposal := migrations.MigrateDealProposal0To1(*legacyProposal)
		proposal = &newProposal
		legacyProtocol = true
	}
	response, err := rv.validatePull(receiver, proposal, legacyProtocol, baseCid, selector)
	if response == nil {
		return nil, err
	}
	if legacyProtocol {
		downgradedResponse := migrations.DealResponse0{
			Status:      response.Status,
			ID:          response.ID,
			Message:     response.Message,
			PaymentOwed: response.PaymentOwed,
		}
		return &downgradedResponse, err
	}
	return response, err
}

func (rv *ProviderRequestValidator) validatePull(receiver peer.ID, proposal *retrievalmarket.DealProposal, legacyProtocol bool, baseCid cid.Cid, selector ipld.Node) (*retrievalmarket.DealResponse, error) {

	if proposal.PayloadCID != baseCid {
		return nil, errors.New("incorrect CID for this proposal")
	}

	buf := new(bytes.Buffer)
	err := dagcbor.Encoder(selector, buf)
	if err != nil {
		return nil, err
	}
	bytesCompare := allSelectorBytes
	if proposal.SelectorSpecified() {
		bytesCompare = proposal.Selector.Raw
	}
	if !bytes.Equal(buf.Bytes(), bytesCompare) {
		return nil, errors.New("incorrect selector for this proposal")
	}

	pds := retrievalmarket.ProviderDealState{
		DealProposal:    *proposal,
		Receiver:        receiver,
		LegacyProtocol:  legacyProtocol,
		CurrentInterval: proposal.PaymentInterval,
	}

	status, err := rv.acceptDeal(&pds)

	response := retrievalmarket.DealResponse{
		ID:     proposal.ID,
		Status: status,
	}

	if status == retrievalmarket.DealStatusFundsNeededUnseal {
		response.PaymentOwed = pds.UnsealPrice
	}

	if err != nil {
		response.Message = err.Error()
		return &response, err
	}

	err = rv.env.BeginTracking(pds)
	if err != nil {
		return nil, err
	}

	return &response, datatransfer.ErrPause
}

func (rv *ProviderRequestValidator) acceptDeal(deal *retrievalmarket.ProviderDealState) (retrievalmarket.DealStatus, error) {
	// check that the deal parameters match our required parameters or
	// reject outright
	err := rv.env.CheckDealParams(deal.PricePerByte, deal.PaymentInterval, deal.PaymentIntervalIncrease, deal.UnsealPrice)
	if err != nil {
		return retrievalmarket.DealStatusRejected, err
	}

	accepted, reason, err := rv.env.RunDealDecisioningLogic(context.TODO(), *deal)
	if err != nil {
		return retrievalmarket.DealStatusErrored, err
	}
	if !accepted {
		return retrievalmarket.DealStatusRejected, errors.New(reason)
	}

	// verify we have the piece
	pieceInfo, err := rv.env.GetPiece(deal.PayloadCID, deal.PieceCID)
	if err != nil {
		if err == retrievalmarket.ErrNotFound {
			return retrievalmarket.DealStatusDealNotFound, err
		}
		return retrievalmarket.DealStatusErrored, err
	}

	deal.PieceInfo = &pieceInfo

	deal.StoreID, err = rv.env.NextStoreID()
	if err != nil {
		return retrievalmarket.DealStatusErrored, err
	}

	if deal.UnsealPrice.GreaterThan(big.Zero()) {
		return retrievalmarket.DealStatusFundsNeededUnseal, nil
	}

	return retrievalmarket.DealStatusAccepted, nil
}
