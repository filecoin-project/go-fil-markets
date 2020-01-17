package retrievalimpl_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/ipfs/go-log/v2"
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
	rcNode1 := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{PayCh: payChAddr})
	client := retrievalimpl.NewClient(nw1, testData.Bs1, rcNode1, &testPeerResolver{})

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
	log.SetDebugLogging()
	bgCtx := context.Background()
	clientPaymentChannel, err := address.NewIDAddress(rand.Uint64())
	require.NoError(t, err)

	testData := tut.NewLibp2pTestData(bgCtx, t)

	// -------- SET UP PROVIDER

	// Inject a unixFS file on the provider side to its blockstore
	// obtained via `ls -laf` on this file

	// pieceLink := testData.LoadUnixFSFile(t, "lorem_big.txt", true)
	// fileSize := uint64(89359)
	pieceLink := testData.LoadUnixFSFile(t, "lorem.txt", true)
	fileSize := uint64(19473)

	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)

	providerNode := testnodes.NewTestRetrievalProviderNode()
	providerNode.SetBlockstore(testData.Bs2)

	expectedQR := tut.MakeTestQueryResponse()

	pieceCID := []byte("pieceCID")
	providerNode.ExpectPiece(pieceCID, expectedQR.Size)

	providerPaymentAddr, err := address.NewIDAddress(rand.Uint64())
	require.NoError(t, err)
	provider := retrievalimpl.NewProvider(providerPaymentAddr, providerNode, nw2)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := &retrievalmarket.RetrievalPeer{Address: providerPaymentAddr, ID: testData.Host2.ID(),}

	// ------- SET UP CLIENT
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1)

	expectedVoucher := tut.MakeTestSignedVoucher()
	expectedTotal := tokenamount.Mul(expectedQR.MinPricePerByte, tokenamount.FromInt(fileSize))
	expectedVoucher.Amount = expectedTotal

	createdChan := make(chan pmtChan)
	paymentChannelRecorder := func(client, miner address.Address, amt tokenamount.TokenAmount) {
		createdChan <- pmtChan{client, miner, amt}
	}

	newLaneAddrChan := make(chan address.Address)
	laneRecorder := func(paymentChannel address.Address) {
		newLaneAddrChan <- paymentChannel
	}

	createdVoucherChan := make(chan *types.SignedVoucher)
	paymentVoucherRecorder := func(v *types.SignedVoucher) {
		createdVoucherChan <- v
	}
	clientNode := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		PayCh:                  clientPaymentChannel,
		Lane:                   3,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,

	})
	client := retrievalimpl.NewClient(nw1, testData.Bs1, clientNode, &testPeerResolver{})

	// **** Send the query for the Piece
	// set up retrieval params
	resp, err := client.Query(bgCtx, *retrievalPeer, pieceCID, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	c, ok := pieceLink.(cidlink.Link)
	require.True(t, ok)
	payloadCID := c.Cid

	rmParams := retrievalmarket.Params{
		PricePerByte:            resp.MinPricePerByte,
		PaymentInterval:         resp.MaxPaymentInterval,
		PaymentIntervalIncrease: resp.MaxPaymentIntervalIncrease,
		PayloadCID:              payloadCID,
	}

	// *** Retrieve the piece
	did := client.Retrieve(bgCtx, pieceCID, rmParams, expectedTotal, retrievalPeer.ID, clientPaymentChannel, retrievalPeer.Address)
	assert.Equal(t, did, retrievalmarket.DealID(1))

	var newChannel pmtChan
	newCtx1, cancel1 := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel1()
	select {
	case <-newCtx1.Done():
		t.Log("channel not created")
		t.FailNow()
	case newChannel = <-createdChan:
	}
	t.Log("here1")
	require.NotNil(t, newChannel)
	require.Equal(t, expectedTotal, newChannel.amt)

	var newLaneAddr address.Address
	newctx2, cancel2 := context.WithTimeout(bgCtx, 30*time.Second)
	defer cancel2()
	select {
	case <-newctx2.Done():
		t.Log("new lane not created")
		t.FailNow()
	case newLaneAddr = <-newLaneAddrChan:
	}
	t.Log("here2")
	require.Equal(t, newLaneAddr, clientPaymentChannel)

	var sawVoucher *types.SignedVoucher
	newCtx, cancel := context.WithTimeout(bgCtx, 30*time.Second)
	defer cancel()
	select {
	case <-newCtx.Done():
		t.Log("voucher not created")
		t.FailNow()
	case sawVoucher = <-createdVoucherChan:
	}
	require.NotNil(t, sawVoucher)
	assert.Equal(t, sawVoucher.Amount, expectedVoucher.Amount)

	// verify that payment channel was created
	// require.NotNil(t, createdChan)
	// assert.True(t, createdChan.amt.Equals(expectedTotal))
	// assert.Equal(t, createdChan.client, clientPaymentChannel)
	// assert.Equal(t, createdChan.miner, providerPaymentAddr)
	// // verify that allocate lane was called
	// require.Len(t, clientNode.lanes, 1)
	// assert.Equal(t, clientNode.lanes[0], true)
	//
	//
	// // verify that the voucher was saved/seen by the client with correct values
	// // verify that the provider saved the same voucher values
	// providerNode.VerifyExpectations(t)
	//
	// require.NotNil(t, sawVoucher)
	// assert.Equal(t, 0, sawVoucher.Lane)
	// assert.True(t, expectedTotal.Equals(sawVoucher.Amount))
	//
	// testData.VerifyFileTransferred(t, pieceLink, false)
}

type pmtChan struct {
	client, miner address.Address
	amt           tokenamount.TokenAmount
}
