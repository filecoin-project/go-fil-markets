package testnodes

import (
	"context"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
)

type TestRetrievalClientNode struct {
	payCh        address.Address
	payChErr     error
	lane         uint64
	laneError    error
	voucher      *types.SignedVoucher
	voucherError error

	allocateLaneRecorder            func(address.Address)
	createPaymentVoucherRecorder    func(voucher *types.SignedVoucher)
	getCreatePaymentChannelRecorder func(address.Address, address.Address, tokenamount.TokenAmount)
}

type TestRetrievalClientNodeParams struct {
	PayCh        address.Address
	PayChErr     error
	Lane         uint64
	LaneError    error
	Voucher      *types.SignedVoucher
	VoucherError error
	AllocateLaneRecorder func(address.Address)
	PaymentVoucherRecorder func(voucher *types.SignedVoucher)
	PaymentChannelRecorder func(address.Address, address.Address, tokenamount.TokenAmount)
}

var _ retrievalmarket.RetrievalClientNode = &TestRetrievalClientNode{}

func NewTestRetrievalClientNode(params TestRetrievalClientNodeParams) *TestRetrievalClientNode {
	return &TestRetrievalClientNode{
		payCh:        params.PayCh,
		payChErr:     params.PayChErr,
		lane:         params.Lane,
		laneError:    params.LaneError,
		voucher:      params.Voucher,
		voucherError: params.VoucherError,
		allocateLaneRecorder: params.AllocateLaneRecorder,
		createPaymentVoucherRecorder: params.PaymentVoucherRecorder,
		getCreatePaymentChannelRecorder: params.PaymentChannelRecorder,
	}
}

func (trcn *TestRetrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	if trcn.getCreatePaymentChannelRecorder != nil {
		trcn.getCreatePaymentChannelRecorder(clientAddress, minerAddress, clientFundsAvailable)
	}
	return trcn.payCh, trcn.payChErr
}

func (trcn *TestRetrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	if trcn.allocateLaneRecorder != nil {
		trcn.allocateLaneRecorder(paymentChannel)
	}
	return trcn.lane, trcn.laneError
}

func (trcn *TestRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	if trcn.createPaymentVoucherRecorder != nil {
		trcn.createPaymentVoucherRecorder(trcn.voucher)
	}
	return trcn.voucher, trcn.voucherError
}

