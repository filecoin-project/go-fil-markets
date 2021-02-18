package retrievalimpl_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	spect "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecemigrations "github.com/filecoin-project/go-fil-markets/piecestore/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestHandleQueryStream(t *testing.T) {
	ctx := context.Background()

	payloadCID := tut.GenerateCids(1)[0]
	expectedPeer := peer.ID("somepeer")
	expectedSize := uint64(1234)

	expectedPieceCID := tut.GenerateCids(1)[0]
	expectedCIDInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPieceCID,
			},
		},
	}
	expectedPiece := piecestore.PieceInfo{
		Deals: []piecestore.DealInfo{
			{
				Length: abi.PaddedPieceSize(expectedSize),
			},
		},
	}
	expectedAddress := address.TestAddress2
	expectedPricePerByte := abi.NewTokenAmount(4321)
	expectedPaymentInterval := uint64(4567)
	expectedPaymentIntervalIncrease := uint64(100)

	readWriteQueryStream := func() network.RetrievalQueryStream {
		qRead, qWrite := tut.QueryReadWriter()
		qrRead, qrWrite := tut.QueryResponseReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespReader: qrRead,
			RespWriter: qrWrite,
		})
		return qs
	}

	receiveStreamOnProvider := func(t *testing.T, qs network.RetrievalQueryStream, pieceStore piecestore.PieceStore) {
		node := testnodes.NewTestRetrievalProviderNode()
		ds := dss.MutexWrap(datastore.NewMapDatastore())
		multiStore, err := multistore.NewMultiDstore(ds)
		require.NoError(t, err)
		dt := tut.NewTestDataTransfer()
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{})
		c, err := retrievalimpl.NewProvider(expectedAddress, node, net, pieceStore, multiStore, dt, ds)
		require.NoError(t, err)
		ask := c.GetAsk()

		ask.PricePerByte = expectedPricePerByte
		ask.PaymentInterval = expectedPaymentInterval
		ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease
		c.SetAsk(ask)

		tut.StartAndWaitForReady(ctx, t, c)

		net.ReceiveQueryStream(qs)
	}

	testCases := []struct {
		name    string
		query   retrievalmarket.Query
		expResp retrievalmarket.QueryResponse
		expErr  string
		expFunc func(t *testing.T, pieceStore *tut.TestPieceStore)
	}{
		{name: "When PieceCID is not provided and PayloadCID is found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore) {
				pieceStore.ExpectCID(payloadCID, expectedCIDInfo)
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)
			},
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
		},
		{name: "When PieceCID is provided and both PieceCID and PayloadCID are found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore) {
				loadPieceCIDS(t, pieceStore, payloadCID, expectedPieceCID)
			},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
		},
		{name: "When QueryParams has PieceCID and is missing",
			expFunc: func(t *testing.T, ps *tut.TestPieceStore) {
				loadPieceCIDS(t, ps, payloadCID, cid.Undef)
				ps.ExpectCID(payloadCID, expectedCIDInfo)
				ps.ExpectMissingPiece(expectedPieceCID)
			},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseUnavailable,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
			},
		},
		{name: "When CID info not found",
			expFunc: func(t *testing.T, ps *tut.TestPieceStore) {
				ps.ExpectMissingCID(payloadCID)
			},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseUnavailable,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			qs := readWriteQueryStream()
			err := qs.WriteQuery(tc.query)
			require.NoError(t, err)
			pieceStore := tut.NewTestPieceStore()
			pieceStore.ExpectCID(payloadCID, expectedCIDInfo)
			pieceStore.ExpectMissingPiece(expectedPieceCID)

			tc.expFunc(t, pieceStore)

			receiveStreamOnProvider(t, qs, pieceStore)

			actualResp, err := qs.ReadQueryResponse()
			pieceStore.VerifyExpectations(t)
			if tc.expErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expErr)
			}

			tc.expResp.PaymentAddress = expectedAddress
			tc.expResp.MinPricePerByte = expectedPricePerByte
			tc.expResp.MaxPaymentInterval = expectedPaymentInterval
			tc.expResp.MaxPaymentIntervalIncrease = expectedPaymentIntervalIncrease
			tc.expResp.UnsealPrice = big.Zero()
			assert.Equal(t, tc.expResp, actualResp)
		})
	}

	t.Run("error reading piece", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: payloadCID,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()

		receiveStreamOnProvider(t, qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseError)
		require.NotEmpty(t, response.Message)
	})

	t.Run("when ReadDealStatusRequest fails", func(t *testing.T) {
		qs := readWriteQueryStream()
		pieceStore := tut.NewTestPieceStore()

		receiveStreamOnProvider(t, qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		require.NotNil(t, err)
		require.Equal(t, response, retrievalmarket.QueryResponseUndefined)
	})

	t.Run("when WriteDealStatusResponse fails", func(t *testing.T) {
		qRead, qWrite := tut.QueryReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespWriter: tut.FailResponseWriter,
		})
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: payloadCID,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		pieceStore.ExpectCID(payloadCID, expectedCIDInfo)
		pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)

		receiveStreamOnProvider(t, qs, pieceStore)

		pieceStore.VerifyExpectations(t)
	})

}

func TestProvider_Construct(t *testing.T) {
	ds := datastore.NewMapDatastore()
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	dt := tut.NewTestDataTransfer()
	_, err = retrievalimpl.NewProvider(
		spect.NewIDAddr(t, 2344),
		testnodes.NewTestRetrievalProviderNode(),
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		tut.NewTestPieceStore(),
		multiStore,
		dt,
		ds,
	)
	require.NoError(t, err)
	require.Len(t, dt.Subscribers, 1)
	require.Len(t, dt.RegisteredVoucherResultTypes, 2)
	_, ok := dt.RegisteredVoucherResultTypes[0].(*retrievalmarket.DealResponse)
	require.True(t, ok)
	_, ok = dt.RegisteredVoucherResultTypes[1].(*migrations.DealResponse0)
	require.True(t, ok)
	require.Len(t, dt.RegisteredVoucherTypes, 2)
	_, ok = dt.RegisteredVoucherTypes[0].VoucherType.(*retrievalmarket.DealProposal)
	require.True(t, ok)
	_, ok = dt.RegisteredVoucherTypes[0].Validator.(*requestvalidation.ProviderRequestValidator)
	require.True(t, ok)
	_, ok = dt.RegisteredVoucherTypes[1].VoucherType.(*migrations.DealProposal0)
	require.True(t, ok)
	_, ok = dt.RegisteredVoucherTypes[1].Validator.(*requestvalidation.ProviderRequestValidator)
	require.True(t, ok)
	require.Len(t, dt.RegisteredRevalidators, 2)
	_, ok = dt.RegisteredRevalidators[0].VoucherType.(*retrievalmarket.DealPayment)
	require.True(t, ok)
	_, ok = dt.RegisteredRevalidators[0].Revalidator.(*requestvalidation.ProviderRevalidator)
	require.True(t, ok)
	_, ok = dt.RegisteredRevalidators[1].VoucherType.(*migrations.DealPayment0)
	require.True(t, ok)
	require.Len(t, dt.RegisteredTransportConfigurers, 2)
	_, ok = dt.RegisteredTransportConfigurers[0].VoucherType.(*retrievalmarket.DealProposal)
	_, ok = dt.RegisteredTransportConfigurers[1].VoucherType.(*migrations.DealProposal0)

	require.True(t, ok)
}
func TestProviderConfigOpts(t *testing.T) {
	var sawOpt int
	opt1 := func(p *retrievalimpl.Provider) { sawOpt++ }
	opt2 := func(p *retrievalimpl.Provider) { sawOpt += 2 }
	ds := datastore.NewMapDatastore()
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	p, err := retrievalimpl.NewProvider(
		spect.NewIDAddr(t, 2344),
		testnodes.NewTestRetrievalProviderNode(),
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		tut.NewTestPieceStore(),
		multiStore,
		tut.NewTestDataTransfer(),
		ds, opt1, opt2,
	)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, 3, sawOpt)

	// just test that we can create a DealDeciderOpt function and that it runs
	// successfully in the constructor
	ddOpt := retrievalimpl.DealDeciderOpt(
		func(_ context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
			return true, "yes", nil
		})

	p, err = retrievalimpl.NewProvider(
		spect.NewIDAddr(t, 2344),
		testnodes.NewTestRetrievalProviderNode(),
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		tut.NewTestPieceStore(),
		multiStore,
		tut.NewTestDataTransfer(),
		ds, ddOpt)
	require.NoError(t, err)
	require.NotNil(t, p)
}

// loadPieceCIDS sets expectations to receive expectedPieceCID and 3 other random PieceCIDs to
// disinguish the case of a PayloadCID is found but the PieceCID is not
func loadPieceCIDS(t *testing.T, pieceStore *tut.TestPieceStore, expPayloadCID, expectedPieceCID cid.Cid) {

	otherPieceCIDs := tut.GenerateCids(3)
	expectedSize := uint64(1234)

	blockLocs := make([]piecestore.PieceBlockLocation, 4)
	expectedPieceInfo := piecestore.PieceInfo{
		PieceCID: expectedPieceCID,
		Deals: []piecestore.DealInfo{
			{
				Length: abi.PaddedPieceSize(expectedSize),
			},
		},
	}

	blockLocs[0] = piecestore.PieceBlockLocation{PieceCID: expectedPieceCID}
	for i, pieceCID := range otherPieceCIDs {
		blockLocs[i+1] = piecestore.PieceBlockLocation{PieceCID: pieceCID}
		pi := expectedPieceInfo
		pi.PieceCID = pieceCID
	}
	if expectedPieceCID != cid.Undef {
		pieceStore.ExpectPiece(expectedPieceCID, expectedPieceInfo)
	}
	expectedCIDInfo := piecestore.CIDInfo{PieceBlockLocations: blockLocs}
	pieceStore.ExpectCID(expPayloadCID, expectedCIDInfo)
}

func TestProviderMigrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	dt := tut.NewTestDataTransfer()

	providerDs := namespace.Wrap(ds, datastore.NewKey("/retrievals/provider"))

	numDeals := 5
	payloadCIDs := make([]cid.Cid, numDeals)
	iDs := make([]retrievalmarket.DealID, numDeals)
	pieceCIDs := make([]*cid.Cid, numDeals)
	pricePerBytes := make([]abi.TokenAmount, numDeals)
	paymentIntervals := make([]uint64, numDeals)
	paymentIntervalIncreases := make([]uint64, numDeals)
	unsealPrices := make([]abi.TokenAmount, numDeals)
	storeIDs := make([]multistore.StoreID, numDeals)
	channelIDs := make([]datatransfer.ChannelID, numDeals)
	receivers := make([]peer.ID, numDeals)
	totalSents := make([]uint64, numDeals)
	messages := make([]string, numDeals)
	currentIntervals := make([]uint64, numDeals)
	fundsReceiveds := make([]abi.TokenAmount, numDeals)
	selfPeer := tut.GeneratePeers(1)[0]
	dealIDs := make([]abi.DealID, numDeals)
	sectorIDs := make([]abi.SectorNumber, numDeals)
	offsets := make([]abi.PaddedPieceSize, numDeals)
	lengths := make([]abi.PaddedPieceSize, numDeals)
	allSelectorBuf := new(bytes.Buffer)
	err = dagcbor.Encoder(shared.AllSelector(), allSelectorBuf)
	require.NoError(t, err)
	allSelectorBytes := allSelectorBuf.Bytes()

	for i := 0; i < numDeals; i++ {
		payloadCIDs[i] = tut.GenerateCids(1)[0]
		iDs[i] = retrievalmarket.DealID(rand.Uint64())
		pieceCID := tut.GenerateCids(1)[0]
		pieceCIDs[i] = &pieceCID
		pricePerBytes[i] = big.NewInt(rand.Int63())
		paymentIntervals[i] = rand.Uint64()
		paymentIntervalIncreases[i] = rand.Uint64()
		unsealPrices[i] = big.NewInt(rand.Int63())
		storeIDs[i] = multistore.StoreID(rand.Uint64())
		receivers[i] = tut.GeneratePeers(1)[0]
		channelIDs[i] = datatransfer.ChannelID{
			Responder: selfPeer,
			Initiator: receivers[i],
			ID:        datatransfer.TransferID(rand.Uint64()),
		}
		totalSents[i] = rand.Uint64()
		messages[i] = string(tut.RandomBytes(20))
		currentIntervals[i] = rand.Uint64()
		fundsReceiveds[i] = big.NewInt(rand.Int63())
		dealIDs[i] = abi.DealID(rand.Uint64())
		sectorIDs[i] = abi.SectorNumber(rand.Uint64())
		offsets[i] = abi.PaddedPieceSize(rand.Uint64())
		lengths[i] = abi.PaddedPieceSize(rand.Uint64())
		deal := migrations.ProviderDealState0{
			DealProposal0: migrations.DealProposal0{
				PayloadCID: payloadCIDs[i],
				ID:         iDs[i],
				Params0: migrations.Params0{
					Selector: &cbg.Deferred{
						Raw: allSelectorBytes,
					},
					PieceCID:                pieceCIDs[i],
					PricePerByte:            pricePerBytes[i],
					PaymentInterval:         paymentIntervals[i],
					PaymentIntervalIncrease: paymentIntervalIncreases[i],
					UnsealPrice:             unsealPrices[i],
				},
			},
			StoreID:   storeIDs[i],
			ChannelID: channelIDs[i],
			PieceInfo: &piecemigrations.PieceInfo0{
				PieceCID: pieceCID,
				Deals: []piecemigrations.DealInfo0{
					{
						DealID:   dealIDs[i],
						SectorID: sectorIDs[i],
						Offset:   offsets[i],
						Length:   lengths[i],
					},
				},
			},
			Status:          retrievalmarket.DealStatusCompleted,
			Receiver:        receivers[i],
			TotalSent:       totalSents[i],
			Message:         messages[i],
			CurrentInterval: currentIntervals[i],
			FundsReceived:   fundsReceiveds[i],
		}
		buf := new(bytes.Buffer)
		err := deal.MarshalCBOR(buf)
		require.NoError(t, err)
		err = providerDs.Put(datastore.NewKey(fmt.Sprint(deal.ID)), buf.Bytes())
		require.NoError(t, err)
	}
	oldAsk := &migrations.Ask0{
		PricePerByte:            abi.NewTokenAmount(rand.Int63()),
		UnsealPrice:             abi.NewTokenAmount(rand.Int63()),
		PaymentInterval:         rand.Uint64(),
		PaymentIntervalIncrease: rand.Uint64(),
	}
	askBuf := new(bytes.Buffer)
	err = oldAsk.MarshalCBOR(askBuf)
	require.NoError(t, err)
	err = providerDs.Put(datastore.NewKey("retrieval-ask"), askBuf.Bytes())
	require.NoError(t, err)
	retrievalProvider, err := retrievalimpl.NewProvider(
		spect.NewIDAddr(t, 2344),
		testnodes.NewTestRetrievalProviderNode(),
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		tut.NewTestPieceStore(),
		multiStore,
		dt,
		providerDs,
	)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, retrievalProvider)
	deals := retrievalProvider.ListDeals()
	require.NoError(t, err)
	for i := 0; i < numDeals; i++ {
		deal, ok := deals[retrievalmarket.ProviderDealIdentifier{Receiver: receivers[i], DealID: iDs[i]}]
		require.True(t, ok)
		expectedDeal := retrievalmarket.ProviderDealState{
			DealProposal: retrievalmarket.DealProposal{
				PayloadCID: payloadCIDs[i],
				ID:         iDs[i],
				Params: retrievalmarket.Params{
					Selector: &cbg.Deferred{
						Raw: allSelectorBytes,
					},
					PieceCID:                pieceCIDs[i],
					PricePerByte:            pricePerBytes[i],
					PaymentInterval:         paymentIntervals[i],
					PaymentIntervalIncrease: paymentIntervalIncreases[i],
					UnsealPrice:             unsealPrices[i],
				},
			},
			StoreID:   storeIDs[i],
			ChannelID: &channelIDs[i],
			PieceInfo: &piecestore.PieceInfo{
				PieceCID: *pieceCIDs[i],
				Deals: []piecestore.DealInfo{
					{
						DealID:   dealIDs[i],
						SectorID: sectorIDs[i],
						Offset:   offsets[i],
						Length:   lengths[i],
					},
				},
			},
			Status:          retrievalmarket.DealStatusCompleted,
			Receiver:        receivers[i],
			TotalSent:       totalSents[i],
			Message:         messages[i],
			CurrentInterval: currentIntervals[i],
			FundsReceived:   fundsReceiveds[i],
			LegacyProtocol:  true,
		}
		require.Equal(t, expectedDeal, deal)
	}
	ask := retrievalProvider.GetAsk()
	expectedAsk := &retrievalmarket.Ask{
		PricePerByte:            oldAsk.PricePerByte,
		UnsealPrice:             oldAsk.UnsealPrice,
		PaymentInterval:         oldAsk.PaymentInterval,
		PaymentIntervalIncrease: oldAsk.PaymentIntervalIncrease,
	}
	require.Equal(t, expectedAsk, ask)
}
