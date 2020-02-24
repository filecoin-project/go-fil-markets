package providerstates

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

// ProviderDealEnvironment is a bridge to the environment a provider deal is executing in
type ProviderDealEnvironment interface {
	Node() rm.RetrievalProviderNode
	GetPieceSize(c cid.Cid) (uint64, error)
	DealStream(id rm.ProviderDealIdentifier) rmnet.RetrievalDealStream
	NextBlock(context.Context, rm.ProviderDealIdentifier) (rm.Block, bool, error)
	CheckDealParams(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) error
}

// ProviderHandlerFunc is a function that handles a provider deal being in a specific state
// It processes the state and returns a modification function for a deal
type ProviderHandlerFunc func(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error

// ReceiveDeal receives and evaluates a deal proposal
func ReceiveDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	dealProposal := deal.DealProposal

	// verify we have the piece
	_, err := environment.GetPieceSize(dealProposal.PayloadCID)
	if err != nil {
		if err == rm.ErrNotFound {
			return ctx.Event(rm.ProviderEventDealNotFound)
		}
		return ctx.Event(rm.ProviderEventGetPieceSizeErrored, err)
	}

	// check that the deal parameters match our required parameters (or reject)
	err = environment.CheckDealParams(dealProposal.PricePerByte, dealProposal.PaymentInterval, dealProposal.PaymentIntervalIncrease)
	if err != nil {
		return ctx.Event(rm.ProviderEventDealRejected, err)
	}

	err = environment.DealStream(deal.Identifier()).WriteDealResponse(rm.DealResponse{
		Status: rm.DealStatusAccepted,
		ID:     deal.ID,
	})
	if err != nil {
		return ctx.Event(rm.ProviderEventWriteResponseFailed, err)
	}

	return ctx.Event(rm.ProviderEventDealAccepted, dealProposal)

}

// SendBlocks sends blocks to the client until funds are needed
func SendBlocks(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	totalSent := deal.TotalSent
	totalPaidFor := big.Div(deal.FundsReceived, deal.PricePerByte).Uint64()
	var blocks []rm.Block

	// read blocks until we reach current interval
	responseStatus := rm.DealStatusFundsNeeded
	for totalSent-totalPaidFor < deal.CurrentInterval {
		block, done, err := environment.NextBlock(ctx.Context(), deal.Identifier())
		if err != nil {
			return ctx.Event(rm.ProviderEventBlockErrored, err)
		}
		blocks = append(blocks, block)
		totalSent += uint64(len(block.Data))
		if done {
			err := ctx.Event(rm.ProviderEventBlocksCompleted)
			if err != nil {
				return err
			}
			responseStatus = rm.DealStatusFundsNeededLastPayment
			break
		}
	}

	// send back response of blocks plus payment owed
	paymentOwed := big.Mul(abi.NewTokenAmount(int64(totalSent-totalPaidFor)), deal.PricePerByte)

	err := environment.DealStream(deal.Identifier()).WriteDealResponse(rm.DealResponse{
		ID:          deal.ID,
		Status:      responseStatus,
		PaymentOwed: paymentOwed,
		Blocks:      blocks,
	})

	if err != nil {
		return ctx.Event(rm.ProviderEventWriteResponseFailed, err)
	}

	return ctx.Event(rm.ProviderEventPaymentRequested, totalSent)
}

// ProcessPayment processes a payment from the client and resumes the deal if successful
func ProcessPayment(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	// read payment, or fail
	payment, err := environment.DealStream(deal.Identifier()).ReadDealPayment()
	if err != nil {
		return ctx.Event(rm.ProviderEventReadPaymentFailed, xerrors.Errorf("reading payment: %w", err))
	}

	// attempt to redeem voucher
	// (totalSent * pricePerbyte) - fundsReceived
	paymentOwed := big.Sub(big.Mul(abi.NewTokenAmount(int64(deal.TotalSent)), deal.PricePerByte), deal.FundsReceived)
	received, err := environment.Node().SavePaymentVoucher(ctx.Context(), payment.PaymentChannel, payment.PaymentVoucher, nil, paymentOwed)
	if err != nil {
		return ctx.Event(rm.ProviderEventSaveVoucherFailed, err)
	}

	// check if all payments are received to continue the deal, or send updated required payment
	if received.LessThan(paymentOwed) {
		err := environment.DealStream(deal.Identifier()).WriteDealResponse(rm.DealResponse{
			ID:          deal.ID,
			Status:      deal.Status,
			PaymentOwed: big.Sub(paymentOwed, received),
		})
		if err != nil {
			return ctx.Event(rm.ProviderEventWriteResponseFailed, err)
		}
		return ctx.Event(rm.ProviderEventPartialPaymentReceived, received)
	}

	// resume deal
	return ctx.Event(rm.ProviderEventPaymentReceived, received)
}

// SendFailResponse sends a failure response before closing the deal
func SendFailResponse(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	stream := environment.DealStream(deal.Identifier())
	err := stream.WriteDealResponse(rm.DealResponse{
		Status:  deal.Status,
		Message: deal.Message,
		ID:      deal.ID,
	})
	if err != nil {
		return ctx.Event(rm.ProviderEventWriteResponseFailed, err)
	}
	return nil
}

// Finalize completes a deal
func Finalize(ctx fsm.Context, environment ProviderDealEnvironment, deal rm.ProviderDealState) error {
	err := environment.DealStream(deal.Identifier()).WriteDealResponse(rm.DealResponse{
		Status: rm.DealStatusCompleted,
		ID:     deal.ID,
	})
	if err != nil {
		return ctx.Event(rm.ProviderEventWriteResponseFailed, err)
	}

	return ctx.Event(rm.ProviderEventComplete)
}
