package clientstates

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
)

// ClientDealEnvironment is a bridge to the environment a client deal is executing in
type ClientDealEnvironment interface {
	Node() rm.RetrievalClientNode
	DealStream() rmnet.RetrievalDealStream
	ConsumeBlock(context.Context, rm.Block) (uint64, bool, error)
}

func errorFunc(err error) func(*rm.ClientDealState) {
	return func(deal *rm.ClientDealState) {
		deal.Status = rm.DealStatusFailed
		deal.Message = err.Error()
	}
}

// ClientHandlerFunc is a function that handles a client deal being in a specific state
// It processes the state and returns a modification function for a deal
type ClientHandlerFunc func(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState)

// SetupPaymentChannel sets up a payment channel for a deal
func SetupPaymentChannel(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {
	paych, err := environment.Node().GetOrCreatePaymentChannel(ctx, deal.ClientWallet, deal.MinerWallet, deal.TotalFunds)
	if err != nil {
		return errorFunc(xerrors.Errorf("getting payment channel: %w", err))
	}
	lane, err := environment.Node().AllocateLane(paych)
	if err != nil {
		return errorFunc(xerrors.Errorf("allocating payment lane: %w", err))
	}
	return func(deal *rm.ClientDealState) {
		deal.Status = rm.DealStatusPaymentChannelCreated
		deal.Message = deal.Message + " SetupPaymentChannel"
		deal.PayCh = paych
		deal.Lane = lane
	}
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {
	stream := environment.DealStream()
	err := stream.WriteDealProposal(deal.DealProposal)
	if err != nil {
		return errorFunc(xerrors.Errorf("proposing deal: %w", err))
	}
	response, err := stream.ReadDealResponse()
	if err != nil {
		return errorFunc(xerrors.Errorf("reading deal response: %w", err))
	}
	if response.Status == rm.DealStatusRejected {
		return func(deal *rm.ClientDealState) {
			deal.Status = rm.DealStatusRejected
			deal.Message = fmt.Sprintf("deal rejected: %s", response.Message)
		}
	}
	if response.Status == rm.DealStatusDealNotFound {
		return func(deal *rm.ClientDealState) {
			deal.Status = rm.DealStatusDealNotFound
			deal.Message = fmt.Sprintf("deal not found: %s", response.Message)
		}
	}
	if response.Status == rm.DealStatusAccepted {
		return func(deal *rm.ClientDealState) {
			deal.Status = rm.DealStatusAccepted
		}
	}
	return errorFunc(xerrors.New("Unexpected deal response status"))
}

// ProcessPaymentRequested processes a request for payment from the provider
func ProcessPaymentRequested(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {

	// check that fundsSpent + paymentRequested <= totalFunds, or fail
	if tokenamount.Add(deal.FundsSpent, deal.PaymentRequested).GreaterThan(deal.TotalFunds) {
		expectedTotal := deal.TotalFunds.String()
		actualTotal := tokenamount.Add(deal.FundsSpent, deal.PaymentRequested).String()
		errMsg := fmt.Sprintf("not enough funds left: expected amt = %s, actual amt = %s", expectedTotal, actualTotal)
		return errorFunc(xerrors.New(errMsg))
	}

	// check that totalReceived - bytesPaidFor >= currentInterval, or fail
	if (deal.TotalReceived-deal.BytesPaidFor < deal.CurrentInterval) && deal.Status != rm.DealStatusFundsNeededLastPayment {
		return errorFunc(xerrors.New("not enough bytes received between payment request"))
	}

	// check that paymentRequest <= (totalReceived - bytesPaidFor) * pricePerByte, or fail
	if deal.PaymentRequested.GreaterThan(tokenamount.Mul(tokenamount.FromInt(deal.TotalReceived-deal.BytesPaidFor), deal.PricePerByte)) {
		return errorFunc(xerrors.New("too much money requested for bytes sent"))
	}
	// create payment voucher with node (or fail) for (fundsSpent + paymentRequested)
	// use correct payCh + lane
	// (node will do subtraction back to paymentRequested... slightly odd behavior but... well anyway)
	voucher, err := environment.Node().CreatePaymentVoucher(ctx, deal.PayCh, tokenamount.Add(deal.FundsSpent, deal.PaymentRequested), deal.Lane)
	if err != nil {
		return errorFunc(xerrors.Errorf("creating payment voucher: %w", err))
	}

	// send payment voucher (or fail)
	err = environment.DealStream().WriteDealPayment(rm.DealPayment{
		ID:             deal.DealProposal.ID,
		PaymentChannel: deal.PayCh,
		PaymentVoucher: voucher,
	})
	if err != nil {
		return errorFunc(xerrors.Errorf("writing deal payment: %w", err))
	}

	// return modify deal function --
	// status = DealStatusOngoing
	// paymentRequested = 0
	// fundsSpent = fundsSpent + paymentRequested
	// if paymentRequested / pricePerByte >= currentInterval
	// currentInterval = currentInterval + proposal.intervalIncrease
	// bytesPaidFor = bytesPaidFor + (paymentRequested / pricePerByte)

	return func(deal *rm.ClientDealState) {
		if deal.Status == rm.DealStatusFundsNeededLastPayment {
			deal.Status = rm.DealStatusCompleted
		} else {
			deal.Status = rm.DealStatusOngoing
		}
		deal.FundsSpent = tokenamount.Add(deal.FundsSpent, deal.PaymentRequested)
		bytesPaidFor := tokenamount.Div(deal.PaymentRequested, deal.PricePerByte).Uint64()
		if bytesPaidFor >= deal.CurrentInterval {
			deal.CurrentInterval += deal.DealProposal.PaymentIntervalIncrease
		}
		deal.BytesPaidFor += bytesPaidFor
		deal.PaymentRequested = tokenamount.FromInt(0)
	}
}

// ProcessNextResponse reads and processes the next response from the provider
func ProcessNextResponse(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {
	// Read next response (or fail)
	response, err := environment.DealStream().ReadDealResponse()
	if err != nil {
		return errorFunc(xerrors.Errorf("reading deal response: %w", err))
	}

	// Process Blocks
	totalProcessed := uint64(0)
	completed := false
	for _, block := range response.Blocks {
		processed, done, err := environment.ConsumeBlock(ctx, block)
		if err != nil {
			return errorFunc(xerrors.Errorf("consuming block: %w", err))
		}
		totalProcessed += processed
		if done {
			completed = true
			break
		}
	}

	// Check For Complete, set completeness
	if completed {
		if response.Status == rm.DealStatusFundsNeededLastPayment {
			return func(deal *rm.ClientDealState) {
				deal.TotalReceived += totalProcessed
				deal.PaymentRequested = response.PaymentOwed
				deal.Status = rm.DealStatusFundsNeededLastPayment
			}
		}
		return func(deal *rm.ClientDealState) {
			deal.TotalReceived += totalProcessed
			deal.Status = rm.DealStatusCompleted
		}
	}

	// Error on complete status, but not all blocks received
	if response.Status == rm.DealStatusFundsNeededLastPayment ||
		response.Status == rm.DealStatusCompleted {
		return errorFunc(xerrors.New("received complete status before all blocks received"))
	}
	// Set PaymentRequested for funds needed statuses
	if response.Status == rm.DealStatusFundsNeeded {
		return func(deal *rm.ClientDealState) {
			deal.TotalReceived += totalProcessed
			deal.PaymentRequested = response.PaymentOwed
			deal.Status = rm.DealStatusFundsNeeded
		}
	}

	// Pass Through Statuses -- retrievalmarket.DealStatusOngoing, retrievalmarket.DealStatusUnsealing
	if response.Status == rm.DealStatusOngoing || response.Status == rm.DealStatusUnsealing {
		return func(deal *rm.ClientDealState) {
			deal.TotalReceived += totalProcessed
			deal.Status = response.Status
		}
	}

	// Error On All Other Statuses
	return errorFunc(xerrors.New("Unexpected deal response status"))
}
