package retrievalimpl_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientsCanTalkToEachOther(t *testing.T) {
	bgCtx := context.Background()
	testData := tut.NewLibp2pTestData(bgCtx, t)

	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)
	payChAddr := address.TestAddress
	rcNode1 := testRetrievalClientNode{payChAddr: payChAddr}
	client := retrievalimpl.NewClient(nw1, testData.Bs1, &rcNode1)

	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	rcNode2 := newTestRetrievalProviderNode()

	expectedCIDs := testutil.GenerateCids(3)
	missingCID := testutil.GenerateCids(1)[0]
	expectedQR := tut.MakeTestQueryResponse()

	rcNode2.expectedMissingPieces[string(missingCID.Bytes())] = struct{}{}
	for i, el := range expectedCIDs {
		key := string(el.Bytes())
		rcNode2.expectedPieces[key] = expectedQR.Size * uint64(i+1)
	}

	paymentAddress := address.TestAddress2
	rcProvider2 := retrievalimpl.NewProvider(paymentAddress, rcNode2, nw2)

	rcProvider2.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	rcProvider2.SetPricePerByte(expectedQR.MinPricePerByte)
	rcProvider2.Start()

	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}

	t.Run("when piece is found, returns piece and price data", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseAvailable
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0].Bytes(), retrievalmarket.QueryParams{})

		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when piece is not found, returns unavailable", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseUnavailable
		expectedQR.Size = 0
		actualQR, err := client.Query(bgCtx, retrievalPeer, missingCID.Bytes(), retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when there is some other error, returns error", func(t *testing.T) {
		unknownPiece := testutil.GenerateCids(1)[0]
		expectedQR.Status = retrievalmarket.QueryResponseError
		expectedQR.Message = "Something went wrong"
		actualQR, err := client.Query(bgCtx, retrievalPeer, unknownPiece.Bytes(), retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})
}

type testRetrievalClientNode struct {
	payChAddr address.Address
	lanes     []bool
}

func (trcn *testRetrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context,
	clientAddress address.Address,
	minerAddress address.Address,
	clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	return trcn.payChAddr, nil
}

func (trcn *testRetrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	trcn.lanes = append(trcn.lanes, true)
	return uint64(len(trcn.lanes) - 1), nil
}

func (trcn *testRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	sv := tut.MakeTestSignedVoucher()
	sv.Amount = amount
	sv.Lane = lane
	return sv, nil
}
