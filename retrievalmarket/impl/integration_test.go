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

	testCases := []struct {
		name     string
		filename string
		filesize uint64
	}{
		{	name: "1 block file retrieval succeeds",
			filename: "lorem_under_1_block.txt",
			filesize: 410},
		{	name:     "multi-block file retrieval succeeds",
			filename: "lorem.txt",
			filesize: 19000},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T){
			pieceLink := testData.LoadUnixFSFile(t, testCase.filename, true)
			pieceCID := []byte("pieceCID")
			providerPaymentAddr, err := address.NewIDAddress(rand.Uint64())
			require.NoError(t, err)
			paymentInterval := uint64(10000)
			paymentIntervalIncrease := uint64(1000)
			pricePerByte := tokenamount.FromInt(1000)

			expectedQR := retrievalmarket.QueryResponse{
				Size:                       1024,
				PaymentAddress:             providerPaymentAddr,
				MinPricePerByte:            pricePerByte,
				MaxPaymentInterval:         paymentInterval,
				MaxPaymentIntervalIncrease: paymentIntervalIncrease,
			}

			providerNode := setupProvider(t, testData, pieceCID, expectedQR, providerPaymentAddr)

			retrievalPeer := &retrievalmarket.RetrievalPeer{Address: providerPaymentAddr, ID: testData.Host2.ID(),}

			expectedVoucher := tut.MakeTestSignedVoucher()

			// just make sure there is enough to cover the transfer
			expectedTotal := tokenamount.Mul(pricePerByte, tokenamount.FromInt(testCase.filesize*2))

			// this is just pulled from the actual answer so the expected keys in the test node match up.
			// later we compare the voucher values.
			expectedVoucher.Amount = tokenamount.FromInt(10136000)
			proof := []byte("")
			require.NoError(t, providerNode.ExpectVoucher(clientPaymentChannel, expectedVoucher, proof, expectedVoucher.Amount, expectedVoucher.Amount, nil))

			// ------- SET UP CLIENT
			nw1 := rmnet.NewFromLibp2pHost(testData.Host1)

			createdChan, newLaneAddr, createdVoucher, client := setupClient(clientPaymentChannel, expectedVoucher, nw1, testData)

			dealStateChan := make(chan retrievalmarket.ClientDealState)
			client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
				switch event {
				case retrievalmarket.ClientEventComplete:
					dealStateChan <- state
				case retrievalmarket.ClientEventError:
					msg := `
Status: %d
TotalReceived: %d
BytesPaidFor: %d
CurrentInterval: %d
TotalFunds: %s
`
					t.Logf(msg, state.Status, state.TotalReceived, state.BytesPaidFor, state.CurrentInterval, state.TotalFunds.String(), )
				}
			})

			// **** Send the query for the Piece
			// set up retrieval params
			resp, err := client.Query(bgCtx, *retrievalPeer, pieceCID, retrievalmarket.QueryParams{})
			require.NoError(t, err)
			require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

			c, ok := pieceLink.(cidlink.Link)
			require.True(t, ok)
			payloadCID := c.Cid

			rmParams := retrievalmarket.Params{
				PricePerByte:            pricePerByte,
				PaymentInterval:         paymentInterval,
				PaymentIntervalIncrease: paymentIntervalIncrease,
				PayloadCID:              payloadCID,
			}

			// *** Retrieve the piece
			did := client.Retrieve(bgCtx, pieceCID, rmParams, expectedTotal, retrievalPeer.ID, clientPaymentChannel, retrievalPeer.Address)
			assert.Equal(t, did, retrievalmarket.DealID(1))

			ctx, cancel := context.WithTimeout(bgCtx, 20*time.Second)
			defer cancel()

			var dealState retrievalmarket.ClientDealState
			select {
			case <-ctx.Done():
				t.Error("deal never completed")
				t.FailNow()
			case dealState = <-dealStateChan:
			}
			assert.Equal(t, dealState.Lane, expectedVoucher.Lane)
			require.NotNil(t, createdChan)
			require.Equal(t, expectedTotal, createdChan.amt)
			require.Equal(t, clientPaymentChannel, *newLaneAddr)
			// verify that the voucher was saved/seen by the client with correct values
			require.NotNil(t, createdVoucher)
			assert.True(t, createdVoucher.Equals(expectedVoucher))
			// // verify that the provider saved the same voucher values
			providerNode.VerifyExpectations(t)
			testData.VerifyFileTransferred(t, pieceLink, false)
		})
	}

}

func setupClient(
	clientPaymentChannel address.Address,
	expectedVoucher *types.SignedVoucher,
	nw1 rmnet.RetrievalMarketNetwork,
	testData *tut.Libp2pTestData) (*pmtChan,
	*address.Address,
	*types.SignedVoucher,
	retrievalmarket.RetrievalClient) {
	var createdChan pmtChan
	paymentChannelRecorder := func(client, miner address.Address, amt tokenamount.TokenAmount) {
		createdChan = pmtChan{client, miner, amt}
	}

	var newLaneAddr address.Address
	laneRecorder := func(paymentChannel address.Address) {
		newLaneAddr = paymentChannel
	}

	var createdVoucher types.SignedVoucher
	paymentVoucherRecorder := func(v *types.SignedVoucher) {
		createdVoucher = *v
	}
	clientNode := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		PayCh:                  clientPaymentChannel,
		Lane:                   expectedVoucher.Lane,
		Voucher:                expectedVoucher,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,
	})
	client := retrievalimpl.NewClient(nw1, testData.Bs1, clientNode, &testPeerResolver{})
	return &createdChan, &newLaneAddr, &createdVoucher, client
}

func setupProvider(t *testing.T, testData *tut.Libp2pTestData, pieceCID []byte, expectedQR retrievalmarket.QueryResponse, providerPaymentAddr address.Address) *testnodes.TestRetrievalProviderNode {
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2)
	providerNode := testnodes.NewTestRetrievalProviderNode()
	providerNode.SetBlockstore(testData.Bs2)
	providerNode.ExpectPiece(pieceCID, expectedQR.Size)
	provider := retrievalimpl.NewProvider(providerPaymentAddr, providerNode, nw2)
	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())
	return providerNode
}

type pmtChan struct {
	client, miner address.Address
	amt           tokenamount.TokenAmount
}
