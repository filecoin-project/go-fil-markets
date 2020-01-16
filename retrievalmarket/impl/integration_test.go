package retrievalimpl_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	payChAddr := address.TestAddress

	client, expectedCIDs, missingPiece, expectedQR, retrievalPeer := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)

	t.Run("when piece is found, returns piece and price data", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseAvailable
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})

		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when piece is not found, returns unavailable", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseUnavailable
		expectedQR.Size = 0
		actualQR, err := client.Query(bgCtx, retrievalPeer, missingPiece, retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when there is some other error, returns error", func(t *testing.T) {
		unknownPiece := testutil.GenerateCids(1)[0]
		expectedQR.Status = retrievalmarket.QueryResponseError
		expectedQR.Message = "GetPieceSize failed"
		actualQR, err := client.Query(bgCtx, retrievalPeer, unknownPiece.Bytes(), retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})
}

func requireSetupTestClientAndProvider(bgCtx context.Context, t *testing.T, payChAddr address.Address) (retrievalmarket.RetrievalClient,
	[][]byte,
	[]byte,
	retrievalmarket.QueryResponse,
	retrievalmarket.RetrievalPeer) {
	testData := tut.NewLibp2pTestData(bgCtx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)
	rcNode1 := testRetrievalClientNode{payChAddr: payChAddr}
	client := retrievalimpl.NewClient(nw1, testData.Bs1, &rcNode1)

	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	providerNode := testnodes.NewTestRetrievalProviderNode()
	providerNode.SetBlockstore(testData.Bs2)

	expectedCIDs := [][]byte{[]byte("piece1"), []byte("piece2"), []byte("piece3")}
	missingPiece := []byte("missingPiece")
	expectedQR := tut.MakeTestQueryResponse()

	providerNode.ExpectMissingPiece(missingPiece)
	for i, piece := range expectedCIDs {
		providerNode.ExpectPiece(piece, expectedQR.Size*uint64(i+1))
	}

	paymentAddress := address.TestAddress2
	provider := retrievalimpl.NewProvider(paymentAddress, providerNode, nw2)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}
	return client, expectedCIDs, missingPiece, expectedQR, retrievalPeer
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	bgCtx := context.Background()
	dealParams := setupDealTest(bgCtx, t)

	createdChan, newLaneAddrChan, createdVoucher := setupChannelsAndRecorders(dealParams)

	// **** Send the query for the Piece
	resp, err := dealParams.client.Query(bgCtx, *dealParams.retrievalPeer, dealParams.pieceCID, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	// set up retrieval params
	c, ok := dealParams.pieceLink.(cidlink.Link)
	require.True(t, ok)

	rmParams := retrievalmarket.Params{
		PricePerByte:            resp.MinPricePerByte,
		PaymentInterval:         resp.MaxPaymentInterval,
		PaymentIntervalIncrease: resp.MaxPaymentIntervalIncrease,
		PayloadCID:              c.Cid,
	}

	expectedTotal := tokenamount.Mul(rmParams.PricePerByte, tokenamount.FromInt(dealParams.fileSize))
	expectedVoucher := tut.MakeTestSignedVoucher()
	expectedVoucher.Amount = expectedTotal

	// *** Retrieve the piece
	did := dealParams.client.Retrieve(bgCtx, dealParams.pieceCID, rmParams, expectedTotal, dealParams.retrievalPeer.ID, dealParams.clientPaymentChannel, dealParams.retrievalPeer.Address)
	assert.Equal(t, did, retrievalmarket.DealID(1))

	var newChannel pmtChan
	newCtx1, cancel1 := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel1()
	select {
	case <-newCtx1.Done():
		t.Error("channel not created")
	case newChannel = <-createdChan:
	}
	require.NotNil(t, newChannel)
	require.Equal(t, expectedTotal, newChannel.amt)

	var newLaneAddr address.Address
	newctx2, cancel2 := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel2()
	select {
	case <-newctx2.Done():
		t.Error("new lane not created")
	case newLaneAddr = <-newLaneAddrChan:
	}
	require.Equal(t, newLaneAddr, dealParams.clientPaymentChannel)

	var sawVoucher *types.SignedVoucher
	newCtx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel()
	select {
	case <-newCtx.Done():
		t.Error("voucher not created")
	case sawVoucher = <-createdVoucher:
	}
	require.NotNil(t, sawVoucher)
	assert.Equal(t, sawVoucher.Amount, expectedVoucher.Amount)

	// verify that payment channel was created
	// require.NotNil(t, createdChan)
	// assert.True(t, createdChan.amt.Equals(expectedTotal))
	// assert.Equal(t, createdChan.client, dealParams.clientPaymentChannel)
	// assert.Equal(t, createdChan.miner, dealParams.providerPaymentAddr)
	// // verify that allocate lane was called
	// require.Len(t, dealParams.clientNode.lanes, 1)
	// assert.Equal(t, dealParams.clientNode.lanes[0], true)
	//
	//
	// // verify that the voucher was saved/seen by the client with correct values
	// // verify that the provider saved the same voucher values
	// dealParams.providerNode.VerifyExpectations(t)
	//
	// require.NotNil(t, sawVoucher)
	// assert.Equal(t, 0, sawVoucher.Lane)
	// assert.True(t, expectedTotal.Equals(sawVoucher.Amount))
	//
	// dealParams.testData.VerifyFileTransferred(t, dealParams.pieceLink, false)
}

type pmtChan struct {
	client, miner address.Address
	amt           tokenamount.TokenAmount
}
func setupChannelsAndRecorders(dealParams dealTestParams) (chan pmtChan, chan address.Address, chan *types.SignedVoucher) {
	createdChan := make(chan pmtChan)
	dealParams.clientNode.getCreatePaymentChannelRecorder = func(client, miner address.Address, amt tokenamount.TokenAmount) {
		createdChan <- pmtChan{client, miner, amt}
	}

	newLaneAddrChan := make(chan address.Address)
	dealParams.clientNode.allocateLaneRecorder = func(paymentChannel address.Address) {
		newLaneAddrChan <- paymentChannel
	}

	createdVoucher := make(chan *types.SignedVoucher)
	dealParams.clientNode.createPaymentVoucherRecorder = func(v *types.SignedVoucher) {
		createdVoucher <- v
	}
	return createdChan, newLaneAddrChan, createdVoucher
}

type dealTestParams struct {
	clientPaymentChannel address.Address
	providerPaymentAddr  address.Address
	testData             *tut.Libp2pTestData
	fileSize			 uint64
	pieceLink            ipld.Link
	pieceCID             []byte
	clientNode           *testRetrievalClientNode
	client               retrievalmarket.RetrievalClient
	providerNode         *testnodes.TestRetrievalProviderNode
	retrievalPeer        *retrievalmarket.RetrievalPeer
}

func setupDealTest(bgCtx context.Context, t *testing.T) dealTestParams {
	payChAddr, err := address.NewIDAddress(rand.Uint64())
	require.NoError(t, err)

	testData := tut.NewLibp2pTestData(bgCtx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)

	link := testData.LoadUnixFSFile(t, "lorem_big.txt", true)
	// ls -laf
	fileSize := uint64(89359)
	linkPiece := []byte(link.String()[:])

	clientNode := &testRetrievalClientNode{payChAddr: payChAddr}
	client := retrievalimpl.NewClient(nw1, testData.Bs1, clientNode)

	// Inject a unixFS file on the provider side to its blockstore
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	providerNode := testnodes.NewTestRetrievalProviderNode()
	providerNode.SetBlockstore(testData.Bs2)

	expectedQR := tut.MakeTestQueryResponse()

	providerNode.ExpectPiece(linkPiece, expectedQR.Size)

	paymentAddress, err := address.NewIDAddress(rand.Uint64())
	require.NoError(t, err)
	provider := retrievalimpl.NewProvider(paymentAddress, providerNode, nw2)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := &retrievalmarket.RetrievalPeer{Address: paymentAddress, ID: testData.Host2.ID(),}
	return dealTestParams{payChAddr, paymentAddress, testData, fileSize, link, linkPiece, clientNode, client, providerNode, retrievalPeer}
}

type testRetrievalClientNode struct {
	payChAddr                       address.Address
	lanes                           []bool
	allocateLaneRecorder            func(address.Address)
	createPaymentVoucherRecorder    func(voucher *types.SignedVoucher)
	getCreatePaymentChannelRecorder func(address.Address, address.Address, tokenamount.TokenAmount)
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
		trcn.createPaymentVoucherRecorder(sv)
	}
	return sv, nil
}
