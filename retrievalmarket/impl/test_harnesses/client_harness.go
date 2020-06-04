package test_harnesses

import (
	"context"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

type ClientHarness struct {
	ctx context.Context
	t   *testing.T
	retrievalmarket.RetrievalClient
	ClientNode                  *testnodes.TestRetrievalClientNode
	CreatedPayChInfo            PayChInfo
	CreatedVoucher              *paych.SignedVoucher
	ExpectedVoucher             *paych.SignedVoucher
	Network                     network.RetrievalMarketNetwork
	NewLaneAddr                 address.Address
	PaychAddr                   address.Address
	CreatePaychCID, AddFundsCID cid.Cid
	TestData                    *shared_testutil.Libp2pTestData
}

type PayChInfo struct {
	Client, Miner address.Address
	Amt           abi.TokenAmount
}

func (ch *ClientHarness) Bootstrap(ctx context.Context, t *testing.T, addFunds bool) *ClientHarness {
	var err error
	ch.t = t
	ch.ctx = ctx
	if ch.TestData == nil {
		ch.TestData = shared_testutil.NewLibp2pTestData(ctx, t)
	}
	paymentChannelRecorder := func(client, miner address.Address, amt abi.TokenAmount) {
		ch.CreatedPayChInfo = PayChInfo{Client: client, Miner: miner, Amt: amt}
	}

	laneRecorder := func(paymentChannel address.Address) {
		ch.NewLaneAddr = paymentChannel
	}

	paymentVoucherRecorder := func(v *paych.SignedVoucher) {
		ch.CreatedVoucher = v
	}
	cids := shared_testutil.GenerateCids(2)
	ch.AddFundsCID = cids[0]
	ch.CreatePaychCID = cids[1]

	if ch.PaychAddr == address.Undef {
		ch.PaychAddr, err = address.NewIDAddress(rand.Uint64())
		require.NoError(ch.t, err)
	}

	if ch.ExpectedVoucher == nil {
		ch.ExpectedVoucher = shared_testutil.MakeTestSignedVoucher()
	}

	if ch.ClientNode == nil {
		ch.ClientNode = testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
			AddFundsOnly:           addFunds,
			PayCh:                  ch.PaychAddr,
			Lane:                   ch.ExpectedVoucher.Lane,
			Voucher:                ch.ExpectedVoucher,
			PaymentChannelRecorder: paymentChannelRecorder,
			AllocateLaneRecorder:   laneRecorder,
			PaymentVoucherRecorder: paymentVoucherRecorder,
			CreatePaychCID:         cids[0],
			AddFundsCID:            cids[1],
		})
	}

	if ch.Network == nil {
		ch.Network = network.NewFromLibp2pHost(ch.TestData.Host1)

	}
	ch.RetrievalClient, err = retrievalimpl.NewClient(ch.Network, ch.TestData.Bs1, ch.ClientNode,
		&shared_testutil.TestPeerResolver{}, ch.TestData.Ds1, ch.TestData.RetrievalStoredCounter1)
	require.NoError(ch.t, err)
	return ch
}
