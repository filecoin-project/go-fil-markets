package testnodes

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
)

// TestRetrievalClientNode is a node adapter for a retrieval client whose responses
// are stubbed
type TestRetrievalClientNode struct {
	payCh        address.Address
	payChErr     error
	lane         int64
	laneError    error
	voucher      *paych.SignedVoucher
	voucherError error

	allocateLaneRecorder            func(address.Address)
	createPaymentVoucherRecorder    func(voucher *paych.SignedVoucher)
	getCreatePaymentChannelRecorder func(address.Address, address.Address, abi.TokenAmount)
}

// TestRetrievalClientNodeParams are parameters for initializing a TestRetrievalClientNode
type TestRetrievalClientNodeParams struct {
	PayCh                  address.Address
	PayChErr               error
	Lane                   int64
	LaneError              error
	Voucher                *paych.SignedVoucher
	VoucherError           error
	AllocateLaneRecorder   func(address.Address)
	PaymentVoucherRecorder func(voucher *paych.SignedVoucher)
	PaymentChannelRecorder func(address.Address, address.Address, abi.TokenAmount)
}

var _ retrievalmarket.RetrievalClientNode = &TestRetrievalClientNode{}

// NewTestRetrievalClientNode instantiates a new TestRetrievalClientNode based ont he given params
func NewTestRetrievalClientNode(params TestRetrievalClientNodeParams) *TestRetrievalClientNode {
	return &TestRetrievalClientNode{
		payCh:                           params.PayCh,
		payChErr:                        params.PayChErr,
		lane:                            params.Lane,
		laneError:                       params.LaneError,
		voucher:                         params.Voucher,
		voucherError:                    params.VoucherError,
		allocateLaneRecorder:            params.AllocateLaneRecorder,
		createPaymentVoucherRecorder:    params.PaymentVoucherRecorder,
		getCreatePaymentChannelRecorder: params.PaymentChannelRecorder,
	}
}

// GetOrCreatePaymentChannel returns a mocked payment channel
func (trcn *TestRetrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable abi.TokenAmount) (address.Address, error) {
	if trcn.getCreatePaymentChannelRecorder != nil {
		trcn.getCreatePaymentChannelRecorder(clientAddress, minerAddress, clientFundsAvailable)
	}
	return trcn.payCh, trcn.payChErr
}

// AllocateLane creates a mock lane on a payment channel
func (trcn *TestRetrievalClientNode) AllocateLane(paymentChannel address.Address) (int64, error) {
	if trcn.allocateLaneRecorder != nil {
		trcn.allocateLaneRecorder(paymentChannel)
	}
	return trcn.lane, trcn.laneError
}

// CreatePaymentVoucher creates a mock payment voucher based on a channel and lane
func (trcn *TestRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount, lane int64) (*paych.SignedVoucher, error) {
	if trcn.createPaymentVoucherRecorder != nil {
		trcn.createPaymentVoucherRecorder(trcn.voucher)
	}
	return trcn.voucher, trcn.voucherError
}
