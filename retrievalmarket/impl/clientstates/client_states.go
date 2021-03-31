package clientstates

import (
	"context"
	"time"

	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

// ClientDealEnvironment is a bridge to the environment a client deal is executing in.
// It provides access to relevant functionality on the retrieval client
type ClientDealEnvironment interface {
	// Node returns the node interface for this deal
	Node() rm.RetrievalClientNode
	OpenDataTransfer(ctx context.Context, to peer.ID, proposal *rm.DealProposal, legacy bool) (datatransfer.ChannelID, error)
	SendDataTransferVoucher(context.Context, datatransfer.ChannelID, *rm.DealPayment, bool) error
	CloseDataTransfer(context.Context, datatransfer.ChannelID) error
	CollectStats(key string, value uint64, average bool)
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	legacy := deal.Status == rm.DealStatusRetryLegacy
	channelID, err := environment.OpenDataTransfer(ctx.Context(), deal.Sender, &deal.DealProposal, legacy)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealProposalErrored, err)
	}
	return ctx.Trigger(rm.ClientEventDealProposed, channelID)
}

// SetupPaymentChannelStart initiates setting up a payment channel for a deal
func SetupPaymentChannelStart(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// If the total funds required for the deal are zero, skip creating the payment channel
	if deal.TotalFunds.IsZero() {
		return ctx.Trigger(rm.ClientEventPaymentChannelSkip)
	}
	t := time.Now()
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
	environment.CollectStats("setup_payment_channel", uint64(time.Since(t).Nanoseconds()), true)
	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, msgCID, paych)
}

// WaitPaymentChannelReady waits for a pending operation on a payment channel -- either creating or depositing funds
func WaitPaymentChannelReady(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	t := time.Now()
	paych, err := environment.Node().WaitForPaymentChannelReady(ctx.Context(), *deal.WaitMsgCID)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	environment.CollectStats("wait_payment_channel", uint64(time.Since(t).Nanoseconds()), true)
	return ctx.Trigger(rm.ClientEventPaymentChannelReady, paych)
}

// AllocateLane allocates a lane for this retrieval operation
func AllocateLane(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	t := time.Now()
	lane, err := environment.Node().AllocateLane(ctx.Context(), deal.PaymentInfo.PayCh)
	if err != nil {
		return ctx.Trigger(rm.ClientEventAllocateLaneErrored, err)
	}
	environment.CollectStats("allocate_lane", uint64(time.Since(t).Nanoseconds()), true)
	return ctx.Trigger(rm.ClientEventLaneAllocated, lane)
}

// Ongoing just double checks that we may need to move out of the ongoing state cause a payment was previously requested
func Ongoing(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	if deal.PaymentRequested.GreaterThan(big.Zero()) {
		if deal.LastPaymentRequested {
			return ctx.Trigger(rm.ClientEventLastPaymentRequested, big.Zero())
		}
		return ctx.Trigger(rm.ClientEventPaymentRequested, big.Zero())
	}
	return nil
}

// ProcessPaymentRequested processes a request for payment from the provider
func ProcessPaymentRequested(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// see if we need to send payment
	if deal.TotalReceived-deal.BytesPaidFor >= deal.CurrentInterval ||
		deal.AllBlocksReceived ||
		deal.UnsealPrice.GreaterThan(deal.UnsealFundsPaid) {
		return ctx.Trigger(rm.ClientEventSendFunds)
	}
	return nil
}

// SendFunds sends the next amount requested by the provider
func SendFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// check that paymentRequest <= (totalReceived - bytesPaidFor) * pricePerByte + (unsealPrice - unsealFundsPaid), or fail
	retrievalPrice := big.Mul(abi.NewTokenAmount(int64(deal.TotalReceived-deal.BytesPaidFor)), deal.PricePerByte)
	unsealPrice := big.Sub(deal.UnsealPrice, deal.UnsealFundsPaid)
	if deal.PaymentRequested.GreaterThan(big.Add(retrievalPrice, unsealPrice)) {
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
		shortfallErr, ok := err.(rm.ShortfallError)
		if ok {
			return ctx.Trigger(rm.ClientEventVoucherShortfall, shortfallErr.Shortfall())
		}
		return ctx.Trigger(rm.ClientEventCreateVoucherFailed, err)
	}

	t := time.Now()
	// send payment voucher (or fail)
	err = environment.SendDataTransferVoucher(ctx.Context(), *deal.ChannelID, &rm.DealPayment{
		ID:             deal.DealProposal.ID,
		PaymentChannel: deal.PaymentInfo.PayCh,
		PaymentVoucher: voucher,
	}, deal.LegacyProtocol)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealPaymentErrored, err)
	}
	environment.CollectStats("send_voucher", uint64(time.Since(t).Nanoseconds()), true)
	environment.CollectStats("num_sent_funds", 1, false)
	// TODO: In case we want to monitor the amount sent in each voucher.
	// environment.CollectStats("amount_send_funds", xxxx , false)
	return ctx.Trigger(rm.ClientEventPaymentSent)
}

// CheckFunds examines current available funds in a payment channel after a voucher shortfall to determine
// a course of action -- whether it's a good time to try again, wait for pending operations, or
// we've truly expended all funds and we need to wait for a manual readd
func CheckFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// if we already have an outstanding operation, let's wait for that to complete
	if deal.WaitMsgCID != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, *deal.WaitMsgCID, deal.PaymentInfo.PayCh)
	}
	availableFunds, err := environment.Node().CheckAvailableFunds(ctx.Context(), deal.PaymentInfo.PayCh)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}
	unredeemedFunds := big.Sub(availableFunds.ConfirmedAmt, availableFunds.VoucherReedeemedAmt)
	shortfall := big.Sub(deal.PaymentRequested, unredeemedFunds)
	if shortfall.LessThanEqual(big.Zero()) {
		return ctx.Trigger(rm.ClientEventPaymentChannelReady, deal.PaymentInfo.PayCh)
	}
	totalInFlight := big.Add(availableFunds.PendingAmt, availableFunds.QueuedAmt)
	if totalInFlight.LessThan(shortfall) || availableFunds.PendingWaitSentinel == nil {
		finalShortfall := big.Sub(shortfall, totalInFlight)
		return ctx.Trigger(rm.ClientEventFundsExpended, finalShortfall)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, *availableFunds.PendingWaitSentinel, deal.PaymentInfo.PayCh)
}

// CancelDeal clears a deal that went wrong for an unknown reason
func CancelDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// If the data transfer has started, cancel it
	if deal.ChannelID != nil {
		// Read next response (or fail)
		err := environment.CloseDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil {
			return ctx.Trigger(rm.ClientEventDataTransferError, err)
		}
	}

	return ctx.Trigger(rm.ClientEventCancelComplete)
}

// CheckComplete verifies that a provider that completed without a last payment requested did in fact send us all the data
func CheckComplete(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// This function is called when the provider tells the client that it has
	// sent all the blocks, so check if all blocks have been received.
	if deal.AllBlocksReceived {
		return ctx.Trigger(rm.ClientEventCompleteVerified)
	}

	// If the deal price per byte is zero, wait for the last blocks to
	// arrive
	if deal.PricePerByte.IsZero() {
		return ctx.Trigger(rm.ClientEventWaitForLastBlocks)
	}

	// If the deal price per byte is non-zero, the provider should only
	// have sent the complete message after receiving the last payment
	// from the client, which should happen after all blocks have been
	// received. So if they haven't been received the provider is trying
	// to terminate the deal early.
	return ctx.Trigger(rm.ClientEventEarlyTermination)
}
