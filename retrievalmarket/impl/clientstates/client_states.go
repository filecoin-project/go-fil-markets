package clientstates

import (
	"context"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("retrieval_clientstates")

// ClientDealEnvironment is a bridge to the environment a client deal is executing in.
// It provides access to relevant functionality on the retrieval client
type ClientDealEnvironment interface {
	// Node returns the node interface for this deal
	Node() rm.RetrievalClientNode
	OpenDataTransfer(ctx context.Context, to peer.ID, proposal *rm.DealProposal) (datatransfer.ChannelID, error)
	SendDataTransferVoucher(context.Context, datatransfer.ChannelID, *rm.DealPayment) error
	CloseDataTransfer(context.Context, datatransfer.ChannelID) error
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	channelID, err := environment.OpenDataTransfer(ctx.Context(), deal.Sender, &deal.DealProposal)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealProposalErrored, err)
	}
	return ctx.Trigger(rm.ClientEventDealProposed, channelID)
}

// SetupPaymentChannelStart initiates setting up a payment channel for a deal
func SetupPaymentChannelStart(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	paych, msgCID, err := environment.Node().GetOrCreatePaymentChannel(ctx.Context(), deal.ClientWallet, deal.MinerWallet, deal.TotalFunds, tok)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	if paych == address.Undef {
		return ctx.Trigger(rm.ClientEventPaymentChannelCreateInitiated, msgCID)
	}

	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, msgCID, paych)
}

// WaitForPaymentChannelCreate waits for payment channel creation to be posted on chain,
//  allocates a lane for vouchers, then signals that the payment channel is ready
func WaitForPaymentChannelCreate(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	paych, err := environment.Node().WaitForPaymentChannelCreation(*deal.WaitMsgCID)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	lane, err := environment.Node().AllocateLane(paych)
	if err != nil {
		return ctx.Trigger(rm.ClientEventAllocateLaneErrored, err)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelReady, paych, lane)
}

// WaitForPaymentChannelAddFunds waits for funds to be added to an existing payment channel, then
// signals that payment channel is ready again
func WaitForPaymentChannelAddFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	err := environment.Node().WaitForPaymentChannelAddFunds(*deal.WaitMsgCID)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelAddFundsErrored, err)
	}
	lane, err := environment.Node().AllocateLane(deal.PaymentInfo.PayCh)
	if err != nil {
		return ctx.Trigger(rm.ClientEventAllocateLaneErrored, err)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelReady, deal.PaymentInfo.PayCh, lane)
}

// Ongoing just double checks that we may need to move out of the ongoing state cause a payment was previously requested
func Ongoing(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	if deal.PaymentRequested.GreaterThan(big.Zero()) {
		log.Error(deal.PaymentRequested)
		if deal.LastPaymentRequested {
			return ctx.Trigger(rm.ClientEventLastPaymentRequested, big.Zero())
		}
		return ctx.Trigger(rm.ClientEventPaymentRequested, big.Zero())
	}
	return nil
}

// ProcessPaymentRequested processes a request for payment from the provider
func ProcessPaymentRequested(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {

	// check that totalReceived - bytesPaidFor >= currentInterval, and send money if we meet that threshold
	if deal.TotalReceived-deal.BytesPaidFor >= deal.CurrentInterval || deal.AllBlocksReceived {
		return ctx.Trigger(rm.ClientEventSendFunds)
	}
	return nil
}

// SendFunds sends the next amount requested by the provider
func SendFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// check that fundsSpent + paymentRequested <= totalFunds, or fail
	if big.Add(deal.FundsSpent, deal.PaymentRequested).GreaterThan(deal.TotalFunds) {
		expectedTotal := deal.TotalFunds.String()
		actualTotal := big.Add(deal.FundsSpent, deal.PaymentRequested).String()
		return ctx.Trigger(rm.ClientEventFundsExpended, expectedTotal, actualTotal)
	}

	// check that paymentRequest <= (totalReceived - bytesPaidFor) * pricePerByte, or fail
	if deal.PaymentRequested.GreaterThan(big.Mul(abi.NewTokenAmount(int64(deal.TotalReceived-deal.BytesPaidFor)), deal.PricePerByte)) {
		return ctx.Trigger(rm.ClientEventBadPaymentRequested, "too much money requested for bytes sent")
	}

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(rm.ClientEventCreateVoucherFailed, err)
	}

	// create payment voucher with node (or fail) for (fundsSpent + paymentRequested)
	// use correct payCh + lane
	// (node will do subtraction back to paymentRequested... slightly odd behavior but... well anyway)
	voucher, err := environment.Node().CreatePaymentVoucher(ctx.Context(), deal.PaymentInfo.PayCh, big.Add(deal.FundsSpent, deal.PaymentRequested), deal.PaymentInfo.Lane, tok)
	if err != nil {
		return ctx.Trigger(rm.ClientEventCreateVoucherFailed, err)
	}

	// send payment voucher (or fail)
	err = environment.SendDataTransferVoucher(ctx.Context(), deal.ChannelID, &rm.DealPayment{
		ID:             deal.DealProposal.ID,
		PaymentChannel: deal.PaymentInfo.PayCh,
		PaymentVoucher: voucher,
	})
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealPaymentErrored, err)
	}

	return ctx.Trigger(rm.ClientEventPaymentSent)
}

// CancelDeal clears a deal that went wrong for an unknown reason
func CancelDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// Read next response (or fail)
	err := environment.CloseDataTransfer(ctx.Context(), deal.ChannelID)
	if err != nil {
		return ctx.Trigger(rm.ClientEventDataTransferError, err)
	}

	return ctx.Trigger(rm.ClientEventCancelComplete)
}
