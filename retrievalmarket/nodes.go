package retrievalmarket

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// RetrievalClientNode are the node dependencies for a RetrievalClient
type RetrievalClientNode interface {
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)

	// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
	// between a client and a miner and ensures the client has the given amount of funds available in the channel
	GetOrCreatePaymentChannel(ctx context.Context, clientAddress, minerAddress address.Address,
		clientFundsAvailable abi.TokenAmount, tok shared.TipSetToken) (address.Address, cid.Cid, error)

	// Allocate late creates a lane within a payment channel so that calls to
	// CreatePaymentVoucher will automatically make vouchers only for the difference
	// in total
	AllocateLane(paymentChannel address.Address) (uint64, error)

	// CreatePaymentVoucher creates a new payment voucher in the given lane for a
	// given payment channel so that all the payment vouchers in the lane add up
	// to the given amount (so the payment voucher will be for the difference)
	CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount,
		lane uint64, tok shared.TipSetToken) (*paych.SignedVoucher, error)

	// WaitForPaymentChannelAddFunds waits for a message on chain that funds have
	// been sent to a payment channel
	WaitForPaymentChannelAddFunds(messageCID cid.Cid) error

	// WaitForPaymentChannelCreation waits for a message on chain that a
	// payment channel has been created
	WaitForPaymentChannelCreation(messageCID cid.Cid) (address.Address, error)
}

// RetrievalProviderNode are the node depedencies for a RetrevalProvider
type RetrievalProviderNode interface {
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)

	// returns the worker address associated with a miner
	GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error)
	UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error)
	SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, tok shared.TipSetToken) (abi.TokenAmount, error)
}
