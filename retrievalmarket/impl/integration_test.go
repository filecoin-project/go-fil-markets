package retrievalimpl_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	payChAddr := address.TestAddress

	client, expectedCIDs, missingCID, expectedQR, retrievalPeer := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)

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

func requireSetupTestClientAndProvider(bgCtx context.Context, t *testing.T, payChAddr address.Address) (retrievalmarket.RetrievalClient, []cid.Cid, cid.Cid, retrievalmarket.QueryResponse, retrievalmarket.RetrievalPeer) {
	testData := tut.NewLibp2pTestData(bgCtx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)
	rcNode1 := testRetrievalClientNode{payChAddr: payChAddr}
	client := retrievalimpl.NewClient(nw1, testData.Bs1, &rcNode1)

	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	rcNode2 := newTestRetrievalProviderNode(&testData.Bs2)

	expectedCIDs := testutil.GenerateCids(3)
	missingCID := testutil.GenerateCids(1)[0]
	expectedQR := tut.MakeTestQueryResponse()

	rcNode2.expectedMissingPieces[string(missingCID.Bytes())] = struct{}{}
	for i, el := range expectedCIDs {
		key := string(el.Bytes())
		rcNode2.expectedPieces[key] = expectedQR.Size * uint64(i+1)
	}

	paymentAddress := address.TestAddress2
	provider := retrievalimpl.NewProvider(paymentAddress, rcNode2, nw2)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}
	return client, expectedCIDs, missingCID, expectedQR, retrievalPeer
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	bgCtx := context.Background()
	payChAddr := address.TestAddress

	testData := tut.NewLibp2pTestData(bgCtx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)

	link := testData.LoadUnixFSFile(t, true)
	linkCidBytes := []byte(link.String()[:])

	clientNode := testRetrievalClientNode{payChAddr: payChAddr}
	client := retrievalimpl.NewClient(nw1, testData.Bs1, &clientNode)

	// Inject a unixFS file on the provider side to its blockstore
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	providerNode := newTestRetrievalProviderNode(&testData.Bs2)

	missingCID := testutil.GenerateCids(1)[0]
	expectedQR := tut.MakeTestQueryResponse()

	providerNode.expectedPieces[link.String()] = expectedQR.Size
	providerNode.expectedMissingPieces[string(missingCID.Bytes())] = struct{}{}

	paymentAddress := address.TestAddress2
	provider := retrievalimpl.NewProvider(paymentAddress, providerNode, nw2)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}

	newLane := make(chan address.Address)
	clientNode.allocateLaneRecorder = func(paymentChannel address.Address) {
		newLane <- paymentChannel
	}

	type voucher struct {
		client address.Address
		funds tokenamount.TokenAmount
		lane uint64
	}
	seenVouchers := make(chan voucher)
	clientNode.createPaymentVoucherRecorder = func(paymentCh address.Address, funds tokenamount.TokenAmount, lane uint64) {
		seenVouchers <- voucher{ paymentCh, funds, lane }
	}

	type pmtChan struct {
		client, miner address.Address
		amt tokenamount.TokenAmount
	}
	createdChan := make(chan pmtChan)
	clientNode.getCreatePaymentChannelRecorder = func(client, miner address.Address, amt tokenamount.TokenAmount) {
		createdChan <- pmtChan{client, miner, amt}
	}

	resp, err :=client.Query(bgCtx, retrievalPeer, linkCidBytes, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	rmParams := retrievalmarket.Params{
		PricePerByte:            resp.MinPricePerByte,
		PaymentInterval:         resp.MaxPaymentInterval,
		PaymentIntervalIncrease: resp.MaxPaymentIntervalIncrease,
	}
	total := tokenamount.TokenAmount{ Int: big.NewInt(9999)}
	did := client.Retrieve(bgCtx, linkCidBytes, rmParams, total, retrievalPeer.ID, payChAddr, retrievalPeer.Address)
	assert.Equal(t, did, retrievalmarket.DealID(1))


	var sawVoucher voucher
	newCtx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel()
	select {
	case <- newCtx.Done():
		t.Error("calls not made")
	case sawVoucher = <- seenVouchers:
	}

	assert.Equal(t, payChAddr, sawVoucher.client)
	assert.Equal(t, paymentAddress, sawVoucher.lane)
	assert.True(t, total.Equals(sawVoucher.funds))

	testData.VerifyFileTransferred(t, link, false)
}


type testRetrievalClientNode struct {
	payChAddr address.Address
	lanes     []bool
	allocateLaneRecorder func(address.Address)
	createPaymentVoucherRecorder func(address.Address, tokenamount.TokenAmount, uint64)
	getCreatePaymentChannelRecorder func(address.Address, address.Address, tokenamount.TokenAmount)
}
func (trcn *testRetrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	trcn.lanes = append(trcn.lanes, true)
	if trcn.allocateLaneRecorder != nil {
		trcn.allocateLaneRecorder(paymentChannel)
	}
	return uint64(len(trcn.lanes) - 1), nil
}


func (trcn *testRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	sv := tut.MakeTestSignedVoucher()
	sv.Amount = amount
	sv.Lane = lane
	if trcn.createPaymentVoucherRecorder != nil {
		trcn.createPaymentVoucherRecorder(paymentChannel, amount, lane)
	}
	return sv, nil
}

func (trcn *testRetrievalClientNode) GetOrCreatePaymentChannel(_ context.Context,
	clientAddress address.Address,
	minerAddress address.Address,
	clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	if trcn.getCreatePaymentChannelRecorder != nil {
		trcn.getCreatePaymentChannelRecorder(clientAddress, minerAddress, clientFundsAvailable)
	}
	return trcn.payChAddr, nil
}
