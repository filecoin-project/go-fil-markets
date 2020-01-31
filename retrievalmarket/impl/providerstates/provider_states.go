package providerstates

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
)

// ProviderDealEnvironment is a bridge to the environment a provider deal is executing in
type ProviderDealEnvironment interface {
	Node() rm.RetrievalProviderNode
	GetPieceSize(c cid.Cid) (uint64, error)
	DealStream() rmnet.RetrievalDealStream
	NextBlock(context.Context) (rm.Block, bool, error)
	CheckDealParams(pricePerByte tokenamount.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) error
}

func errorFunc(err error) func(*rm.ProviderDealState) {
	return func(deal *rm.ProviderDealState) {
		deal.Status = rm.DealStatusFailed
		deal.Message = err.Error()
	}
}

func responseFailure(stream rmnet.RetrievalDealStream, status rm.DealStatus, message string, id rm.DealID) func(*rm.ProviderDealState) {
	err := stream.WriteDealResponse(rm.DealResponse{
		Status:  status,
		Message: message,
		ID:      id,
	})
	if err != nil {
		return errorFunc(xerrors.Errorf("writing deal response: %w", err))
	}
	return func(deal *rm.ProviderDealState) {
		deal.Status = status
		deal.Message = message
	}
}

// ProviderHandlerFunc is a function that handles a provider deal being in a specific state
// It processes the state and returns a modification function for a deal
type ProviderHandlerFunc func(ctx context.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) func(*rm.ProviderDealState)

// ReceiveDeal receives and evaluates a deal proposal
func ReceiveDeal(ctx context.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) func(*rm.ProviderDealState) {
	// read deal proposal (or fail)
	dealProposal, err := environment.DealStream().ReadDealProposal()
	if err != nil {
		return errorFunc(xerrors.Errorf("reading deal proposal: %w", err))
	}

	// verify we have the piece
	_, err = environment.GetPieceSize(dealProposal.PayloadCID)
	if err != nil {
		if err == rm.ErrNotFound {
			return responseFailure(environment.DealStream(), rm.DealStatusDealNotFound, rm.ErrNotFound.Error(), dealProposal.ID)
		}
		return responseFailure(environment.DealStream(), rm.DealStatusFailed, err.Error(), dealProposal.ID)
	}

	// check that the deal parameters match our required parameters (or reject)
	err = environment.CheckDealParams(dealProposal.PricePerByte, dealProposal.PaymentInterval, dealProposal.PaymentIntervalIncrease)
	if err != nil {
		return responseFailure(environment.DealStream(), rm.DealStatusRejected, err.Error(), dealProposal.ID)
	}

	// accept the deal
	err = environment.DealStream().WriteDealResponse(rm.DealResponse{
		Status: rm.DealStatusAccepted,
		ID:     dealProposal.ID,
	})
	if err != nil {
		return errorFunc(xerrors.Errorf("writing real response: %w", err))
	}

	// update that we are ready to start sending blocks
	return func(deal *rm.ProviderDealState) {
		deal.Status = rm.DealStatusAccepted
		deal.CurrentInterval = dealProposal.PaymentInterval
		deal.DealProposal = dealProposal
	}
}

// SendBlocks sends blocks to the client until funds are needed
func SendBlocks(ctx context.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) func(*rm.ProviderDealState) {
	totalSent := deal.TotalSent
	totalPaidFor := tokenamount.Div(deal.FundsReceived, deal.PricePerByte).Uint64()
	returnStatus := rm.DealStatusFundsNeeded
	var blocks []rm.Block

	// read blocks until we reach current interval
	for totalSent-totalPaidFor < deal.CurrentInterval {
		block, done, err := environment.NextBlock(ctx)
		if err != nil {
			return responseFailure(environment.DealStream(), rm.DealStatusFailed, err.Error(), deal.ID)
		}
		blocks = append(blocks, block)
		totalSent += uint64(len(block.Data))
		if done {
			returnStatus = rm.DealStatusFundsNeededLastPayment
			break
		}
	}
	// send back response of blocks plus payment owed
	paymentOwed := tokenamount.Mul(tokenamount.FromInt(totalSent-totalPaidFor), deal.PricePerByte)
	err := environment.DealStream().WriteDealResponse(rm.DealResponse{
		ID:          deal.ID,
		Status:      returnStatus,
		PaymentOwed: paymentOwed,
		Blocks:      blocks,
	})
	if err != nil {
		return errorFunc(xerrors.Errorf("writing deal response: %w", err))
	}

	// set status to awaiting funds and update amount sent
	return func(deal *rm.ProviderDealState) {
		deal.Status = returnStatus
		deal.TotalSent = totalSent
	}
}

// ProcessPayment processes a payment from the client and resumes the deal if successful
func ProcessPayment(ctx context.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) func(*rm.ProviderDealState) {
	// read payment, or fail
	payment, err := environment.DealStream().ReadDealPayment()
	if err != nil {
		return errorFunc(xerrors.Errorf("reading payment: %w", err))
	}

	// attempt to redeem voucher
	// (totalSent * pricePerbyte) - fundsReceived
	paymentOwed := tokenamount.Sub(tokenamount.Mul(tokenamount.FromInt(deal.TotalSent), deal.PricePerByte), deal.FundsReceived)
	received, err := environment.Node().SavePaymentVoucher(ctx, payment.PaymentChannel, payment.PaymentVoucher, nil, paymentOwed)
	if err != nil {
		return responseFailure(environment.DealStream(), rm.DealStatusFailed, err.Error(), deal.ID)
	}

	// check if all payments are received to continue the deal, or send updated required payment
	if received.LessThan(paymentOwed) {
		err := environment.DealStream().WriteDealResponse(rm.DealResponse{
			ID:          deal.ID,
			Status:      deal.Status,
			PaymentOwed: tokenamount.Sub(paymentOwed, received),
		})
		if err != nil {
			return errorFunc(xerrors.Errorf("writing deal response", err))
		}
		return func(deal *rm.ProviderDealState) {
			deal.FundsReceived = tokenamount.Add(deal.FundsReceived, received)
		}
	}

	// resume deal
	return func(deal *rm.ProviderDealState) {
		if deal.Status == rm.DealStatusFundsNeededLastPayment {
			deal.Status = rm.DealStatusCompleted
		} else {
			deal.Status = rm.DealStatusOngoing
		}
		deal.FundsReceived = tokenamount.Add(deal.FundsReceived, received)
		deal.CurrentInterval += deal.PaymentIntervalIncrease
	}
}
