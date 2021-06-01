package retrievalimpl_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/network"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/pieceio/cario"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	"github.com/filecoin-project/go-data-transfer/testutil"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	rmtesting "github.com/filecoin-project/go-fil-markets/retrievalmarket/testing"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	payChAddr := address.TestAddress

	client, expectedCIDs, missingPiece, expectedQR, retrievalPeer, _ := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)

	t.Run("when piece is found, returns piece and price data", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseAvailable
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})

		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when piece is not found, returns unavailable", func(t *testing.T) {
		expectedQR.PieceCIDFound = retrievalmarket.QueryItemUnavailable
		expectedQR.Status = retrievalmarket.QueryResponseUnavailable
		expectedQR.Size = 0
		actualQR, err := client.Query(bgCtx, retrievalPeer, missingPiece, retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when there is some other error, returns error", func(t *testing.T) {
		unknownPiece := tut.GenerateCids(1)[0]
		expectedQR.Status = retrievalmarket.QueryResponseError
		expectedQR.Message = "get cid info: GetCIDInfo failed"
		actualQR, err := client.Query(bgCtx, retrievalPeer, unknownPiece, retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

}

func TestProvider_Stop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	bgCtx := context.Background()
	payChAddr := address.TestAddress
	client, expectedCIDs, _, _, retrievalPeer, provider := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)
	require.NoError(t, provider.Stop())
	_, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})

	assert.EqualError(t, err, "exhausted 5 attempts but failed to open stream, err: protocol not supported")
}

func requireSetupTestClientAndProvider(ctx context.Context, t *testing.T, payChAddr address.Address) (retrievalmarket.RetrievalClient,
	[]cid.Cid,
	cid.Cid,
	retrievalmarket.QueryResponse,
	retrievalmarket.RetrievalPeer,
	retrievalmarket.RetrievalProvider) {
	testData := tut.NewLibp2pTestData(ctx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1, rmnet.RetryParameters(100*time.Millisecond, 1*time.Second, 5, 5))
	cids := tut.GenerateCids(2)
	rcNode1 := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		PayCh:          payChAddr,
		CreatePaychCID: cids[0],
		AddFundsCID:    cids[1],
	})

	gs1 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host1), testData.Loader1, testData.Storer1)
	dtTransport1 := dtgstransport.NewTransport(testData.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(testData.DTStore1, testData.DTTmpDir1, testData.DTNet1, dtTransport1)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt1)
	require.NoError(t, err)
	clientDs := namespace.Wrap(testData.Ds1, datastore.NewKey("/retrievals/client"))
	client, err := retrievalimpl.NewClient(nw1, testData.MultiStore1, dt1, rcNode1, &tut.TestPeerResolver{}, clientDs)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, client)
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2, rmnet.RetryParameters(0, 0, 0, 0))
	providerNode := testnodes.NewTestRetrievalProviderNode()
	pieceStore := tut.NewTestPieceStore()
	expectedCIDs := tut.GenerateCids(3)
	expectedPieceCIDs := tut.GenerateCids(3)
	missingCID := tut.GenerateCids(1)[0]
	expectedQR := tut.MakeTestQueryResponse()

	pieceStore.ExpectMissingCID(missingCID)
	for i, c := range expectedCIDs {
		pieceStore.ExpectCID(c, piecestore.CIDInfo{
			PieceBlockLocations: []piecestore.PieceBlockLocation{
				{
					PieceCID: expectedPieceCIDs[i],
				},
			},
		})
	}
	for i, piece := range expectedPieceCIDs {
		pieceStore.ExpectPiece(piece, piecestore.PieceInfo{
			Deals: []piecestore.DealInfo{
				{
					Length: abi.PaddedPieceSize(expectedQR.Size * uint64(i+1)),
				},
			},
		})
	}

	paymentAddress := address.TestAddress2

	gs2 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host2), testData.Loader2, testData.Storer2)
	dtTransport2 := dtgstransport.NewTransport(testData.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(testData.DTStore2, testData.DTTmpDir2, testData.DTNet2, dtTransport2)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt2)
	require.NoError(t, err)
	providerDs := namespace.Wrap(testData.Ds2, datastore.NewKey("/retrievals/provider"))
	provider, err := retrievalimpl.NewProvider(paymentAddress, providerNode, nw2, pieceStore, testData.MultiStore2, dt2, providerDs)
	require.NoError(t, err)

	ask := provider.GetAsk()
	ask.PaymentInterval = expectedQR.MaxPaymentInterval
	ask.PaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
	ask.PricePerByte = expectedQR.MinPricePerByte
	ask.UnsealPrice = expectedQR.UnsealPrice
	provider.SetAsk(ask)
	tut.StartAndWaitForReady(ctx, t, provider)
	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}
	rcNode1.ExpectKnownAddresses(retrievalPeer, nil)
	return client, expectedCIDs, missingCID, expectedQR, retrievalPeer, provider
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	// -------- SET UP PROVIDER

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

	partialSelector := ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
		specBuilder.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
			specBuilder.Insert("Hash", ssb.Matcher())
		})))
	}).Node()

	var customDeciderRan bool

	testCases := []struct {
		name                    string
		decider                 retrievalimpl.DealDecider
		filename                string
		filesize                uint64
		voucherAmts             []abi.TokenAmount
		selector                ipld.Node
		unsealPrice             abi.TokenAmount
		zeroPricePerByte        bool
		paramsV1, addFunds      bool
		skipStores              bool
		failsUnseal             bool
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		channelAvailableFunds   retrievalmarket.ChannelAvailableFunds
		fundsReplenish          abi.TokenAmount
		cancelled               bool
		disableNewDeals         bool
	}{
		{name: "1 block file retrieval succeeds",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			addFunds:    false,
		},
		{name: "1 block file retrieval succeeds with unseal price",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			unsealPrice: abi.NewTokenAmount(100),
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(100), abi.NewTokenAmount(410100)},
			selector:    shared.AllSelector(),
			paramsV1:    true,
		},
		{name: "1 block file retrieval succeeds with existing payment channel",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			addFunds:    true},
		{name: "1 block file retrieval succeeds, but waits for other payment channel funds to land",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall and then waiting for both
				// the pending and then the queued amounts
				ConfirmedAmt:        abi.NewTokenAmount(-410000),
				PendingAmt:          abi.NewTokenAmount(200000),
				PendingWaitSentinel: &tut.GenerateCids(1)[0],
				QueuedAmt:           abi.NewTokenAmount(210000),
			},
		},
		{name: "1 block file retrieval succeeds, after insufficient funds and restart",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall
				ConfirmedAmt: abi.NewTokenAmount(-410000),
			},
			fundsReplenish: abi.NewTokenAmount(410000),
		},
		{name: "1 block file retrieval cancelled after insufficient funds",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall
				ConfirmedAmt: abi.NewTokenAmount(-410000),
			},
			cancelled: true,
		},
		{name: "multi-block file retrieval succeeds",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(19920000)},
		},
		{name: "multi-block file retrieval with zero price per byte succeeds",
			filename:         "lorem.txt",
			filesize:         19000,
			zeroPricePerByte: true,
		},
		{name: "multi-block file retrieval succeeds with V1 params and AllSelector",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(19920000)},
			paramsV1:    true,
			selector:    shared.AllSelector()},
		{name: "partial file retrieval succeeds with V1 params and selector recursion depth 1",
			filename:    "lorem.txt",
			filesize:    1024,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(1944000)},
			paramsV1:    true,
			selector:    partialSelector},
		{name: "succeeds when using a custom decider function",
			decider: func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
				customDeciderRan = true
				return true, "", nil
			},
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
		},
		{name: "succeeds for regular blockstore",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(19920000)},
			skipStores:  true,
		},
		{
			name:        "failed unseal",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{},
			failsUnseal: true,
		},
		{name: "multi-block file retrieval succeeds, final block exceeds payment interval",
			filename:                "lorem.txt",
			filesize:                19000,
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(9112000), abi.NewTokenAmount(19352000), abi.NewTokenAmount(19920000)},
			paymentInterval:         9000,
			paymentIntervalIncrease: 1250,
		},
		{name: "multi-block file retrieval succeeds, final block lands on payment interval",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(9112000), abi.NewTokenAmount(19920000)},
			// Total bytes: 19,920
			// intervals: 9,000 | 9,000 + (9,000 + 1920)
			paymentInterval:         9000,
			paymentIntervalIncrease: 1920,
		},
		{name: "multi-block file retrieval succeeds, with provider only accepting legacy deals",
			filename:        "lorem.txt",
			filesize:        19000,
			disableNewDeals: true,
			voucherAmts:     []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(19920000)},
		},
	}

	for i, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bgCtx := context.Background()
			clientPaymentChannel, err := address.NewIDAddress(uint64(i * 10))
			require.NoError(t, err)

			testData := tut.NewLibp2pTestData(bgCtx, t)

			// Inject a unixFS file on the provider side to its blockstore
			// obtained via `ls -laf` on this file

			fpath := filepath.Join("retrievalmarket", "impl", "fixtures", testCase.filename)

			pieceLink, storeID := testData.LoadUnixFSFileToStore(t, fpath, true)
			c, ok := pieceLink.(cidlink.Link)
			require.True(t, ok)
			payloadCID := c.Cid
			providerPaymentAddr, err := address.NewIDAddress(uint64(i * 99))
			require.NoError(t, err)
			paymentInterval := testCase.paymentInterval
			if paymentInterval == 0 {
				paymentInterval = uint64(10000)
			}
			paymentIntervalIncrease := testCase.paymentIntervalIncrease
			if paymentIntervalIncrease == 0 {
				paymentIntervalIncrease = uint64(1000)
			}
			pricePerByte := abi.NewTokenAmount(1000)
			if testCase.zeroPricePerByte {
				pricePerByte = abi.NewTokenAmount(0)
			}
			unsealPrice := testCase.unsealPrice
			if unsealPrice.Int == nil {
				unsealPrice = big.Zero()
			}

			expectedQR := retrievalmarket.QueryResponse{
				Size:                       1024,
				PaymentAddress:             providerPaymentAddr,
				MinPricePerByte:            pricePerByte,
				MaxPaymentInterval:         paymentInterval,
				MaxPaymentIntervalIncrease: paymentIntervalIncrease,
				UnsealPrice:                unsealPrice,
			}

			providerNode := testnodes.NewTestRetrievalProviderNode()
			var pieceInfo piecestore.PieceInfo
			cio := cario.NewCarIO()
			var buf bytes.Buffer
			store, err := testData.MultiStore2.Get(storeID)
			require.NoError(t, err)
			err = cio.WriteCar(bgCtx, store.Bstore, payloadCID, shared.AllSelector(), &buf)
			require.NoError(t, err)
			carData := buf.Bytes()
			sectorID := abi.SectorNumber(100000)
			offset := abi.PaddedPieceSize(1000)
			pieceInfo = piecestore.PieceInfo{
				PieceCID: tut.GenerateCids(1)[0],
				Deals: []piecestore.DealInfo{
					{
						SectorID: sectorID,
						Offset:   offset,
						Length:   abi.UnpaddedPieceSize(len(carData)).Padded(),
					},
				},
			}
			if testCase.failsUnseal {
				providerNode.ExpectFailedUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(len(carData)))
			} else {
				providerNode.ExpectUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(len(carData)), carData)
			}

			// clearout provider blockstore
			err = testData.MultiStore2.Delete(storeID)
			require.NoError(t, err)

			decider := rmtesting.TrivialTestDecider
			if testCase.decider != nil {
				decider = testCase.decider
			}

			// ------- SET UP CLIENT
			ctx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
			defer cancel()

			provider := setupProvider(bgCtx, t, testData, payloadCID, pieceInfo, expectedQR,
				providerPaymentAddr, providerNode, decider, testCase.disableNewDeals)
			tut.StartAndWaitForReady(ctx, t, provider)

			retrievalPeer := retrievalmarket.RetrievalPeer{Address: providerPaymentAddr, ID: testData.Host2.ID()}

			expectedVoucher := tut.MakeTestSignedVoucher()

			// just make sure there is enough to cover the transfer
			expectedTotal := big.Mul(pricePerByte, abi.NewTokenAmount(int64(len(carData))))

			// voucherAmts are pulled from the actual answer so the expected keys in the test node match up.
			// later we compare the voucher values.  The last voucherAmt is a remainder
			proof := []byte("")
			for _, voucherAmt := range testCase.voucherAmts {
				require.NoError(t, providerNode.ExpectVoucher(clientPaymentChannel, expectedVoucher, proof, voucherAmt, voucherAmt, nil))
			}

			nw1 := rmnet.NewFromLibp2pHost(testData.Host1, rmnet.RetryParameters(0, 0, 0, 0))
			createdChan, newLaneAddr, createdVoucher, clientNode, client, err := setupClient(bgCtx, t, clientPaymentChannel, expectedVoucher, nw1, testData, testCase.addFunds, testCase.channelAvailableFunds)
			require.NoError(t, err)
			tut.StartAndWaitForReady(ctx, t, client)

			clientNode.ExpectKnownAddresses(retrievalPeer, nil)

			clientDealStateChan := make(chan retrievalmarket.ClientDealState)
			client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted, retrievalmarket.DealStatusCancelled, retrievalmarket.DealStatusErrored:
					clientDealStateChan <- state
					return
				}
				if state.Status == retrievalmarket.DealStatusInsufficientFunds {
					if !testCase.fundsReplenish.Nil() {
						clientNode.ResetChannelAvailableFunds(retrievalmarket.ChannelAvailableFunds{
							ConfirmedAmt: testCase.fundsReplenish,
						})
						client.TryRestartInsufficientFunds(state.PaymentInfo.PayCh)
					}
					if testCase.cancelled {
						client.CancelDeal(state.ID)
					}
				}
				msg := `
Client:
Event:           %s
Status:          %s
TotalReceived:   %d
BytesPaidFor:    %d
CurrentInterval: %d
TotalFunds:      %s
Message:         %s
`
				t.Logf(msg, retrievalmarket.ClientEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
					state.TotalFunds.String(), state.Message)
			})

			providerDealStateChan := make(chan retrievalmarket.ProviderDealState)
			provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted, retrievalmarket.DealStatusCancelled, retrievalmarket.DealStatusErrored:
					providerDealStateChan <- state
					return
				}
				msg := `
Provider:
Event:           %s
Status:          %s
TotalSent:       %d
FundsReceived:   %s
Message:		 %s
CurrentInterval: %d
`
				t.Logf(msg, retrievalmarket.ProviderEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
					state.CurrentInterval)

			})
			// **** Send the query for the Piece
			// set up retrieval params
			resp, err := client.Query(bgCtx, retrievalPeer, payloadCID, retrievalmarket.QueryParams{})
			require.NoError(t, err)
			require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

			var rmParams retrievalmarket.Params
			if testCase.paramsV1 {
				rmParams, err = retrievalmarket.NewParamsV1(pricePerByte, paymentInterval, paymentIntervalIncrease, testCase.selector, nil, unsealPrice)
				require.NoError(t, err)
			} else {
				rmParams = retrievalmarket.NewParamsV0(pricePerByte, paymentInterval, paymentIntervalIncrease)
			}

			var clientStoreID *multistore.StoreID
			if !testCase.skipStores {
				id := testData.MultiStore1.Next()
				clientStoreID = &id
			}
			// *** Retrieve the piece
			_, err = client.Retrieve(bgCtx, payloadCID, rmParams, expectedTotal, retrievalPeer, clientPaymentChannel, retrievalPeer.Address, clientStoreID)
			require.NoError(t, err)

			// verify that client subscribers will be notified of state changes
			var clientDealState retrievalmarket.ClientDealState
			select {
			case <-ctx.Done():
				t.Error("deal never completed")
				t.FailNow()
			case clientDealState = <-clientDealStateChan:
			}
			if testCase.failsUnseal || testCase.cancelled {
				assert.Equal(t, retrievalmarket.DealStatusCancelled, clientDealState.Status)
			} else {
				if !testCase.zeroPricePerByte {
					assert.Equal(t, clientDealState.PaymentInfo.Lane, expectedVoucher.Lane)
					require.NotNil(t, createdChan)
					require.Equal(t, expectedTotal, createdChan.amt)
					require.Equal(t, clientPaymentChannel, *newLaneAddr)

					// verify that the voucher was saved/seen by the client with correct values
					require.NotNil(t, createdVoucher)
					tut.TestVoucherEquality(t, createdVoucher, expectedVoucher)
				}
				assert.Equal(t, retrievalmarket.DealStatusCompleted, clientDealState.Status)
			}
			ctx, cancel = context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			var providerDealState retrievalmarket.ProviderDealState
			select {
			case <-ctx.Done():
				t.Error("provider never saw completed deal")
				t.FailNow()
			case providerDealState = <-providerDealStateChan:
			}

			if testCase.failsUnseal {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusErrored, providerDealState.Status)
			} else if testCase.cancelled {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusCancelled, providerDealState.Status)
			} else {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusCompleted, providerDealState.Status)
			}
			// TODO this is terrible, but it's temporary until the test harness refactor
			// in the resuming retrieval deals branch is done
			// https://github.com/filecoin-project/go-fil-markets/issues/65
			if testCase.decider != nil {
				assert.True(t, customDeciderRan)
			}
			// verify that the nodes we interacted with as expected
			clientNode.VerifyExpectations(t)
			providerNode.VerifyExpectations(t)
			if !testCase.failsUnseal && !testCase.cancelled {
				if testCase.skipStores {
					testData.VerifyFileTransferred(t, pieceLink, false, testCase.filesize)
				} else {
					testData.VerifyFileTransferredIntoStore(t, pieceLink, *clientStoreID, false, testCase.filesize)
				}
			}
		})
	}

}

func setupClient(
	ctx context.Context,
	t *testing.T,
	clientPaymentChannel address.Address,
	expectedVoucher *paych.SignedVoucher,
	nw1 rmnet.RetrievalMarketNetwork,
	testData *tut.Libp2pTestData,
	addFunds bool,
	channelAvailableFunds retrievalmarket.ChannelAvailableFunds,
) (
	*pmtChan,
	*address.Address,
	*paych.SignedVoucher,
	*testnodes.TestRetrievalClientNode,
	retrievalmarket.RetrievalClient,
	error) {
	var createdChan pmtChan
	paymentChannelRecorder := func(client, miner address.Address, amt abi.TokenAmount) {
		createdChan = pmtChan{client, miner, amt}
	}

	var newLaneAddr address.Address
	laneRecorder := func(paymentChannel address.Address) {
		newLaneAddr = paymentChannel
	}

	var createdVoucher paych.SignedVoucher
	paymentVoucherRecorder := func(v *paych.SignedVoucher) {
		createdVoucher = *v
	}
	cids := tut.GenerateCids(2)
	clientNode := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		AddFundsOnly:           addFunds,
		PayCh:                  clientPaymentChannel,
		Lane:                   expectedVoucher.Lane,
		Voucher:                expectedVoucher,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,
		CreatePaychCID:         cids[0],
		AddFundsCID:            cids[1],
		IntegrationTest:        true,
		ChannelAvailableFunds:  channelAvailableFunds,
	})

	gs1 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host1), testData.Loader1, testData.Storer1)
	dtTransport1 := dtgstransport.NewTransport(testData.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(testData.DTStore1, testData.DTTmpDir1, testData.DTNet1, dtTransport1)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt1)
	require.NoError(t, err)
	clientDs := namespace.Wrap(testData.Ds1, datastore.NewKey("/retrievals/client"))

	client, err := retrievalimpl.NewClient(nw1, testData.MultiStore1, dt1, clientNode, &tut.TestPeerResolver{}, clientDs)
	return &createdChan, &newLaneAddr, &createdVoucher, clientNode, client, err
}

func setupProvider(
	ctx context.Context,
	t *testing.T,
	testData *tut.Libp2pTestData,
	payloadCID cid.Cid,
	pieceInfo piecestore.PieceInfo,
	expectedQR retrievalmarket.QueryResponse,
	providerPaymentAddr address.Address,
	providerNode retrievalmarket.RetrievalProviderNode,
	decider retrievalimpl.DealDecider,
	disableNewDeals bool,
) retrievalmarket.RetrievalProvider {
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2, rmnet.RetryParameters(0, 0, 0, 0))
	pieceStore := tut.NewTestPieceStore()
	expectedPiece := tut.GenerateCids(1)[0]
	cidInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPiece,
			},
		},
	}
	pieceStore.ExpectCID(payloadCID, cidInfo)
	pieceStore.ExpectPiece(expectedPiece, pieceInfo)

	gs2 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host2), testData.Loader2, testData.Storer2)
	dtTransport2 := dtgstransport.NewTransport(testData.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(testData.DTStore2, testData.DTTmpDir2, testData.DTNet2, dtTransport2)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt2)
	require.NoError(t, err)
	providerDs := namespace.Wrap(testData.Ds2, datastore.NewKey("/retrievals/provider"))

	opts := []retrievalimpl.RetrievalProviderOption{retrievalimpl.DealDeciderOpt(decider)}
	if disableNewDeals {
		opts = append(opts, retrievalimpl.DisableNewDeals())
	}
	provider, err := retrievalimpl.NewProvider(providerPaymentAddr, providerNode, nw2,
		pieceStore, testData.MultiStore2, dt2, providerDs,
		opts...)
	require.NoError(t, err)

	ask := provider.GetAsk()

	ask.PaymentInterval = expectedQR.MaxPaymentInterval
	ask.PaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
	ask.PricePerByte = expectedQR.MinPricePerByte
	ask.UnsealPrice = expectedQR.UnsealPrice
	provider.SetAsk(ask)
	return provider
}

type pmtChan struct {
	client, miner address.Address
	amt           abi.TokenAmount
}
