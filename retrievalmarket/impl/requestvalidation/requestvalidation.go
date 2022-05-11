package requestvalidation

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	peer "github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("markets-rtvl-reval")

var allSelectorBytes []byte

var askTimeout = 5 * time.Second

func init() {
	buf := new(bytes.Buffer)
	_ = dagcbor.Encode(selectorparse.CommonSelector_ExploreAllRecursively, buf)
	allSelectorBytes = buf.Bytes()
}

// ValidationEnvironment contains the dependencies needed to validate deals
type ValidationEnvironment interface {
	GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid, piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (rm.Ask, error)

	GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error)
	// CheckDealParams verifies the given deal params are acceptable
	CheckDealParams(ask rm.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error
	// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
	RunDealDecisioningLogic(ctx context.Context, state rm.ProviderDealState) (bool, string, error)
	// StateMachines returns the FSM Group to begin tracking with
	BeginTracking(pds rm.ProviderDealState) error
	Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error)
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
func (rv *ProviderRequestValidator) ValidatePush(_ datatransfer.ChannelID, sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ValidationResult, error) {
	return datatransfer.ValidationResult{}, errors.New("No pushes accepted")
}

// ValidatePull validates a pull request received from the peer that will receive data
func (rv *ProviderRequestValidator) ValidatePull(_ datatransfer.ChannelID, receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ValidationResult, error) {
	proposal, ok := voucher.(*rm.DealProposal)
	if !ok {
		return datatransfer.ValidationResult{}, errors.New("wrong voucher type")
	}

	response, err := rv.validatePull(receiver, proposal, baseCid, selector)
	return response, err
}

func rejectProposal(proposal *rm.DealProposal, status rm.DealStatus, reason string) datatransfer.ValidationResult {
	return datatransfer.ValidationResult{
		Accepted: false,
		VoucherResult: &rm.DealResponse{
			ID:      proposal.ID,
			Status:  status,
			Message: reason,
		},
	}
}

// validatePull is called by the data provider when a new graphsync pull
// request is created. This can be the initial pull request or a new request
// created when the data transfer is restarted (eg after a connection failure).
// By default the graphsync request starts immediately sending data, unless
// validatePull returns ErrPause or the data-transfer has not yet started
// (because the provider is still unsealing the data).
func (rv *ProviderRequestValidator) validatePull(receiver peer.ID, proposal *rm.DealProposal, baseCid cid.Cid, selector ipld.Node) (datatransfer.ValidationResult, error) {
	// Check the proposal CID matches
	if proposal.PayloadCID != baseCid {
		return rejectProposal(proposal, rm.DealStatusRejected, "incorrect CID for this proposal"), nil
	}

	// Check the proposal selector matches
	buf := new(bytes.Buffer)
	err := dagcbor.Encode(selector, buf)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusRejected, err.Error()), nil
	}
	bytesCompare := allSelectorBytes
	if proposal.SelectorSpecified() {
		bytesCompare = proposal.Selector.Raw
	}
	if !bytes.Equal(buf.Bytes(), bytesCompare) {
		return rejectProposal(proposal, rm.DealStatusRejected, "incorrect selector specified for this proposal"), nil
	}

	// This is a new graphsync request (not a restart)
	deal := rm.ProviderDealState{
		DealProposal: *proposal,
		Receiver:     receiver,
	}

	pieceInfo, isUnsealed, err := rv.env.GetPiece(deal.PayloadCID, deal.PieceCID)
	if err != nil {
		if err == rm.ErrNotFound {
			return rejectProposal(proposal, rm.DealStatusDealNotFound, err.Error()), nil
		}
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error()), nil
	}

	ctx, cancel := context.WithTimeout(context.TODO(), askTimeout)
	defer cancel()

	ask, err := rv.env.GetAsk(ctx, deal.PayloadCID, deal.PieceCID, pieceInfo, isUnsealed, deal.Receiver)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error()), nil
	}

	// check that the deal parameters match our required parameters or
	// reject outright
	err = rv.env.CheckDealParams(ask, deal.PricePerByte, deal.PaymentInterval, deal.PaymentIntervalIncrease, deal.UnsealPrice)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusRejected, err.Error()), nil
	}

	accepted, reason, err := rv.env.RunDealDecisioningLogic(context.TODO(), deal)
	if err != nil {
		return rejectProposal(proposal, rm.DealStatusErrored, err.Error()), nil
	}
	if !accepted {
		return rejectProposal(proposal, rm.DealStatusRejected, reason), nil
	}

	deal.PieceInfo = &pieceInfo

	err = rv.env.BeginTracking(deal)
	if err != nil {
		return datatransfer.ValidationResult{}, err
	}

	status := rm.DealStatusAccepted
	if deal.UnsealPrice.GreaterThan(big.Zero()) {
		status = rm.DealStatusFundsNeededUnseal
	}
	// Pause the data transfer while unsealing the data.
	// The state machine will unpause the transfer when unsealing completes.
	result := datatransfer.ValidationResult{
		Accepted: true,
		VoucherResult: &rm.DealResponse{
			ID:          proposal.ID,
			Status:      status,
			PaymentOwed: deal.Params.OutstandingBalance(big.Zero(), 0, false),
		},
		ForcePause:           true,
		DataLimit:            deal.Params.NextInterval(big.Zero()),
		RequiresFinalization: true,
	}
	return result, nil
}

// ValidateRestart validates a request on restart, based on its current state
func (rv *ProviderRequestValidator) ValidateRestart(channelID datatransfer.ChannelID, channelState datatransfer.ChannelState) (datatransfer.ValidationResult, error) {
	proposal, ok := channelState.Voucher().(*rm.DealProposal)
	if !ok {
		return datatransfer.ValidationResult{}, errors.New("wrong voucher type")
	}

	dealID := rm.ProviderDealIdentifier{DealID: proposal.ID, Receiver: channelState.OtherPeer()}

	deal, err := rv.env.Get(dealID)
	if err != nil {
		return errorDealResponse(dealID, err), nil
	}

	// check if all payments are received to continue the deal, or send updated required payment
	owed := deal.Params.OutstandingBalance(deal.FundsReceived, channelState.Queued(), channelState.Status().InFinalization())
	log.Debugf("provider: owed %d: total received %d, total sent %d, unseal price %d, price per byte %d",
		owed, deal.FundsReceived, channelState.Sent(), deal.UnsealPrice, deal.PricePerByte)

	return datatransfer.ValidationResult{
		Accepted:             true,
		ForcePause:           deal.Status == rm.DealStatusUnsealing || deal.Status == rm.DealStatusFundsNeededUnseal,
		RequiresFinalization: owed.GreaterThan(big.Zero()) || (deal.Status != rm.DealStatusFundsNeededLastPayment && deal.Status != rm.DealStatusFinalizing),
		DataLimit:            deal.Params.NextInterval(deal.FundsReceived),
	}, nil
}

func errorDealResponse(dealID rm.ProviderDealIdentifier, err error) datatransfer.ValidationResult {
	return datatransfer.ValidationResult{
		Accepted: false,
		VoucherResult: &rm.DealResponse{
			ID:      dealID.DealID,
			Message: err.Error(),
			Status:  rm.DealStatusErrored,
		},
	}
}
