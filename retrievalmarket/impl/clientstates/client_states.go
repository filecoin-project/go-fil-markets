package clientstates

import (
	"context"
	"fmt"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"
)

// ClientDealEnvironment is a bridge to the environment a client deal is executing in
type ClientDealEnvironment interface {
	Node() rm.RetrievalClientNode
	DealStream() rmnet.RetrievalDealStream
	Blockstore() blockstore.Blockstore
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
		deal.PayCh = paych
		deal.Lane = lane
	}
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {
	stream := environment.DealStream()
	err := stream.WriteDealProposal(deal.DealProposal)
	if err != nil {
		return errorFunc(err)
	}
	response, err := stream.ReadDealResponse()
	if err != nil {
		return errorFunc(err)
	}
	if response.Status == rm.DealStatusRejected {
		return func(deal *rm.ClientDealState) {
			deal.Status = rm.DealStatusRejected
			deal.Message = fmt.Sprintf("deal rejected: %w", response.Message)
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

	// check that fundsSpent + paymentRequested < totalFunds, or fail

	// check that totalReceived - bytesPaidFor >= currentInterval, or fail

	// check that paymentRequest <= (totalReceived - bytesPaidFor) * pricePerByte, or fail

	// create payment voucher with node (or fail) for (fundsSpent + paymentRequested)
	// use correct payCh + lane
	// (node will do subtraction back to paymentRequested... slightly odd behavior but... well anyway)

	// send payment voucher (or fail)

	// return modify deal function --
	// status = DealStatusOngoing
	// paymentRequested = 0
	// fundsSpent = fundsSpent + paymentRequested
	// if paymentRequested / pricePerByte >= currentInterval
	// currentInterval = currentInterval + proposal.intervalIncrease
	// bytesPaidFor = bytesPaidFor + (paymentRequested / pricePerByte)

	return func(*rm.ClientDealState) {}
}

// ProcessNextResponse reads and processes the next response from the provider
func ProcessNextResponse(ctx context.Context, environment ClientDealEnvironment, deal rm.ClientDealState) func(*rm.ClientDealState) {
	return func(*rm.ClientDealState) {}
}
