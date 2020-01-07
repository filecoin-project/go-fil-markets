package testnodes

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-components/shared/tokenamount"
	"github.com/filecoin-project/go-fil-components/shared/types"
)

type TestRetrievalClientNode struct {
	payCh        address.Address
	payChErr     error
	lane         uint64
	laneError    error
	voucher      *types.SignedVoucher
	voucherError error
}

type TestRetrievalClientNodeParams struct {
	PayCh        address.Address
	PayChErr     error
	Lane         uint64
	LaneError    error
	Voucher      *types.SignedVoucher
	VoucherError error
}

func NewTestRetrievalClientNode(params TestRetrievalClientNodeParams) *TestRetrievalClientNode {
	return &TestRetrievalClientNode{
		payCh:        params.PayCh,
		payChErr:     params.PayChErr,
		lane:         params.Lane,
		laneError:    params.LaneError,
		voucher:      params.Voucher,
		voucherError: params.VoucherError,
	}
}

func (t *TestRetrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	return t.payCh, t.payChErr
}

func (t *TestRetrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	return t.lane, t.laneError
}

func (t *TestRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	return t.voucher, t.voucherError
}
