package clientstates

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
)

// ClientDealEnvironment is a bridge to the environment a client deal is executing in
type ClientDealEnvironment interface {
	Node() rm.RetrievalClientNode
	DealStream(id rm.DealID) rmnet.RetrievalDealStream
	ConsumeBlock(context.Context, rm.DealID, rm.Block) (uint64, bool, error)
}

// SetupPaymentChannel sets up a payment channel for a deal
func SetupPaymentChannel(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	paych, msgCID, err := environment.Node().GetOrCreatePaymentChannel(ctx.Context(), deal.ClientWallet, deal.MinerWallet, deal.TotalFunds, tok)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}
	if paych == address.Undef  {
		return ctx.Trigger(rm.ClientEventPaymentChannelCreateInitiated)
	}
	//lane, err := environment.Node().AllocateLane(paych)
	//if err != nil {
	//	return ctx.Trigger(rm.ClientEventAllocateLaneErrored, err)
	//}
	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, paych, msgCID)
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	stream := environment.DealStream(deal.ID)
	err := stream.WriteDealProposal(deal.DealProposal)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealProposalErrored, err)
	}
	response, err := stream.ReadDealResponse()
	if err != nil {
		return ctx.Trigger(rm.ClientEventReadDealResponseErrored, err)
	}
	switch response.Status {
	case rm.DealStatusRejected:
		return ctx.Trigger(rm.ClientEventDealRejected, response.Message)
	case rm.DealStatusDealNotFound:
		return ctx.Trigger(rm.ClientEventDealNotFound, response.Message)
	case rm.DealStatusAccepted:
		return ctx.Trigger(rm.ClientEventDealAccepted)
	default:
		return ctx.Trigger(rm.ClientEventUnknownResponseReceived)
	}
}

// ProcessPaymentRequested processes a request for payment from the provider
func ProcessPaymentRequested(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// check that fundsSpent + paymentRequested <= totalFunds, or fail
	if big.Add(deal.FundsSpent, deal.PaymentRequested).GreaterThan(deal.TotalFunds) {
		expectedTotal := deal.TotalFunds.String()
		actualTotal := big.Add(deal.FundsSpent, deal.PaymentRequested).String()
		return ctx.Trigger(rm.ClientEventFundsExpended, expectedTotal, actualTotal)
	}

	// check that totalReceived - bytesPaidFor >= currentInterval, or fail
	if (deal.TotalReceived-deal.BytesPaidFor < deal.CurrentInterval) && deal.Status != rm.DealStatusFundsNeededLastPayment {
		return ctx.Trigger(rm.ClientEventBadPaymentRequested, "not enough bytes received between payment request")
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
	err = environment.DealStream(deal.ID).WriteDealPayment(rm.DealPayment{
		ID:             deal.DealProposal.ID,
		PaymentChannel: deal.PaymentInfo.PayCh,
		PaymentVoucher: voucher,
	})
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealPaymentErrored, err)
	}

	return ctx.Trigger(rm.ClientEventPaymentSent)
}

// ProcessNextResponse reads and processes the next response from the provider
func ProcessNextResponse(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// Read next response (or fail)
	response, err := environment.DealStream(deal.ID).ReadDealResponse()
	if err != nil {
		return ctx.Trigger(rm.ClientEventReadDealResponseErrored, err)
	}

	// Process Blocks
	totalProcessed := uint64(0)
	completed := deal.Status == rm.DealStatusBlocksComplete
	if !completed {
		var processed uint64
		for _, block := range response.Blocks {
			processed, completed, err = environment.ConsumeBlock(ctx.Context(), deal.ID, block)
			if err != nil {
				return ctx.Trigger(rm.ClientEventConsumeBlockFailed, err)
			}
			totalProcessed += processed
			if completed {
				break
			}
		}
	}

	if completed {
		switch response.Status {
		case rm.DealStatusFundsNeededLastPayment:
			return ctx.Trigger(rm.ClientEventLastPaymentRequested, totalProcessed, response.PaymentOwed)
		case rm.DealStatusBlocksComplete:
			return ctx.Trigger(rm.ClientEventAllBlocksReceived, totalProcessed)
		case rm.DealStatusCompleted:
			return ctx.Trigger(rm.ClientEventComplete, totalProcessed)
		default:
			return ctx.Trigger(rm.ClientEventUnknownResponseReceived)
		}
	}
	switch response.Status {
	// Error on complete status, but not all blocks received
	case rm.DealStatusFundsNeededLastPayment, rm.DealStatusCompleted:
		return ctx.Trigger(rm.ClientEventEarlyTermination)
	case rm.DealStatusFundsNeeded:
		return ctx.Trigger(rm.ClientEventPaymentRequested, totalProcessed, response.PaymentOwed)
	case rm.DealStatusOngoing:
		return ctx.Trigger(rm.ClientEventBlocksReceived, totalProcessed)
	default:
		return ctx.Trigger(rm.ClientEventUnknownResponseReceived)
	}
}

// Finalize completes a deal
func Finalize(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// Read next response (or fail)
	response, err := environment.DealStream(deal.ID).ReadDealResponse()
	if err != nil {
		return ctx.Trigger(rm.ClientEventReadDealResponseErrored, err)
	}

	if response.Status != rm.DealStatusCompleted {
		return ctx.Trigger(rm.ClientEventUnknownResponseReceived)
	}

	return ctx.Trigger(rm.ClientEventComplete, uint64(0))
}
