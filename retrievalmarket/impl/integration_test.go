package retrievalimpl_test

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/test_harnesses"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	rmtesting "github.com/filecoin-project/go-fil-markets/retrievalmarket/testing"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	client, expectedCIDs, missingPiece, expectedQR, retrievalPeer, _ := requireSetupTestClientAndProvider(bgCtx, t)

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
	bgCtx := context.Background()
	client, expectedCIDs, _, _, retrievalPeer, provider := requireSetupTestClientAndProvider(bgCtx, t)
	require.NoError(t, provider.Stop())
	_, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})
	assert.EqualError(t, err, "protocol not supported")
}

func requireSetupTestClientAndProvider(bgCtx context.Context, t *testing.T) (retrievalmarket.RetrievalClient,
	[]cid.Cid,
	cid.Cid,
	retrievalmarket.QueryResponse,
	retrievalmarket.RetrievalPeer,
	retrievalmarket.RetrievalProvider) {

	ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, false)

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
					Length: expectedQR.Size * uint64(i+1),
				},
			},
		})
	}

	paymentAddress := address.TestAddress2
	nw2 := rmnet.NewFromLibp2pHost(ch.TestData.Host2)
	provider, err := retrievalimpl.NewProvider(paymentAddress, providerNode, nw2,
		pieceStore, ch.TestData.Bs2, ch.TestData.Ds2)
	require.NoError(t, err)

	provider.SetPaymentInterval(expectedQR.MaxPaymentInterval, expectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(expectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())

	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      ch.TestData.Host2.ID(),
	}
	return ch.RetrievalClient, expectedCIDs, missingCID, expectedQR, retrievalPeer, provider
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)

	partialSelector := ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
		specBuilder.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
			specBuilder.Insert("Hash", ssb.Matcher())
		})))
	}).Node()

	var customDeciderRan bool

	testCases := []struct {
		name                          string
		decider                       retrievalimpl.DealDecider
		filename                      string
		filesize                      uint64
		voucherAmts                   []abi.TokenAmount
		selector                      ipld.Node
		paramsV1, unsealing, addFunds bool
	}{
		{name: "1 block file retrieval succeeds",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)}},
		{name: "1 block file retrieval succeeds with existing payment channel",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			addFunds:    true},
		{name: "1 block file retrieval succeeds with unsealing",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			unsealing:   true},
		{name: "multi-block file retrieval succeeds",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)}},
		{name: "multi-block file retrieval succeeds with unsealing",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)},
			unsealing:   true},
		{name: "multi-block file retrieval succeeds with V1 params and AllSelector",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)},
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
			unsealing:   false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bgCtx := context.Background()

			// ------- SET UP CLIENT
			ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, testCase.addFunds)

			clientDealStateChan := make(chan retrievalmarket.ClientDealState)
			ch.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
				switch event {
				case retrievalmarket.ClientEventComplete:
					clientDealStateChan <- state
				default:
					logClientDealState(t, state)
				}
			})

			ph := new(test_harnesses.ProviderHarness)
			ph.TestData = ch.TestData
			ph.PaychAddr = ch.PaychAddr
			// 			decider := rmtesting.TrivalTestDecider
			//			if testCase.decider != nil {
			//				decider = testCase.decider
			//			}
			ph.Bootstrap(bgCtx, t, testCase.filename, testCase.filesize, testCase.unsealing)

			expQR := ph.ExpectedQR
			retrievalPeer := &retrievalmarket.RetrievalPeer{
				Address: expQR.PaymentAddress, ID: ph.TestData.Host2.ID(),
			}

			// just make sure there is enough to cover the transfer
			expectedTotal := big.Mul(expQR.MinPricePerByte, abi.NewTokenAmount(int64(testCase.filesize*2)))

			// voucherAmts are pulled from the actual answer so the expected keys in the test node match up.
			// later we compare the voucher values.  The last voucherAmt is a remainder
			proof := []byte("")
			for _, voucherAmt := range testCase.voucherAmts {
				require.NoError(t, ph.ProviderNode.ExpectVoucher(ph.PaychAddr, ch.ExpectedVoucher, proof, voucherAmt, voucherAmt, nil))
			}

			providerDealStateChan := make(chan retrievalmarket.ProviderDealState)
			ph.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
				switch event {
				case retrievalmarket.ProviderEventComplete:
					providerDealStateChan <- state
				default:
					logProviderDealState(t, state)
				}
			})

			// **** Send the query for the Piece
			// set up retrieval params
			resp, err := ch.Query(bgCtx, *retrievalPeer, ph.PayloadCID, retrievalmarket.QueryParams{})
			require.NoError(t, err)
			require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

			var rmParams retrievalmarket.Params
			if testCase.paramsV1 {
				rmParams = retrievalmarket.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
					expQR.MaxPaymentIntervalIncrease, testCase.selector, nil)

			} else {
				rmParams = retrievalmarket.NewParamsV0(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
					expQR.MaxPaymentIntervalIncrease)
			}

			// *** Retrieve the piece, simulating a shutdown in between
			did, err := ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
				ph.PaychAddr, retrievalPeer.Address)
			require.NoError(t, err)

			require.Equal(t, did, retrievalmarket.DealID(0))

			// verify that client subscribers will be notified of state changes
			ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			var clientDealState retrievalmarket.ClientDealState
			select {
			case <-ctx.Done():
				t.Error("deal never completed")
				t.FailNow()
			case clientDealState = <-clientDealStateChan:
			}
			assert.Equal(t, clientDealState.PaymentInfo.Lane, ch.ExpectedVoucher.Lane)
			require.NotNil(t, ch.CreatedPayChInfo)
			require.Equal(t, expectedTotal, ch.CreatedPayChInfo.Amt)
			require.Equal(t, ph.PaychAddr, ch.NewLaneAddr)
			// verify that the voucher was saved/seen by the client with correct values
			require.NotNil(t, ch.CreatedVoucher)
			tut.TestVoucherEquality(t, ch.CreatedVoucher, ch.ExpectedVoucher)

			ctx, cancel = context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			var providerDealState retrievalmarket.ProviderDealState
			select {
			case <-ctx.Done():
				t.Error("provider never saw completed deal")
				t.FailNow()
			case providerDealState = <-providerDealStateChan:
			}

			assert.Equal(t, int(retrievalmarket.DealStatusCompleted), int(providerDealState.Status))

			// TODO this is terrible, but it's temporary until the test harness refactor
			// in the resuming retrieval deals branch is done
			// https://github.com/filecoin-project/go-fil-markets/issues/65
			if testCase.decider != nil {
				assert.True(t, customDeciderRan)
			}
			// verify that the provider saved the same voucher values
			ph.ProviderNode.VerifyExpectations(t)
			ph.TestData.VerifyFileTransferred(t, ph.PieceLink, false, testCase.filesize)
		})
	}

}

/// =======================
func TestStartStopProvider(t *testing.T) {
	filename := "lorem.txt"
	filesize := uint64(19000)
	voucherAmts := []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)}

	bgCtx := context.Background()
	ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, false)

	ph := new(test_harnesses.ProviderHarness)
	ph.TestData = ch.TestData
	ph.PaychAddr = ch.PaychAddr
	ph.Bootstrap(bgCtx, t, filename, filesize, false)

	expQR := ph.ExpectedQR
	retrievalPeer := &retrievalmarket.RetrievalPeer{
		Address: expQR.PaymentAddress, ID: ph.TestData.Host2.ID(),
	}
	expectedTotal := big.Mul(expQR.MinPricePerByte, abi.NewTokenAmount(int64(filesize*2)))

	proof := []byte("")
	for _, voucherAmt := range voucherAmts {
		require.NoError(t, ph.ProviderNode.ExpectVoucher(ph.PaychAddr, ch.ExpectedVoucher,
			proof, voucherAmt, voucherAmt, nil))
	}
	// **** Send the query for the Piece
	// set up retrieval params
	resp, err := ch.Query(bgCtx, *retrievalPeer, ph.PayloadCID, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	clientDealStateChan := make(chan retrievalmarket.ClientDealState)
	ch.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
		switch event {
		case retrievalmarket.ClientEventComplete:
			clientDealStateChan <- state
		}
	})

	rmParams := retrievalmarket.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
		expQR.MaxPaymentIntervalIncrease, shared.AllSelector(), nil)
	// *** Retrieve the piece, simulating a shutdown in between
	did, err := ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
		ch.PaychAddr, retrievalPeer.Address)
	require.NoError(t, err)

	require.NoError(t, ph.Stop())

	provider2, err := retrievalimpl.NewProvider(ph.PaychAddr, ph.ProviderNode, ph.Network, ph.PieceStore,
		ph.TestData.Bs2, ph.TestData.Ds2)
	require.NoError(t, err)
	provider2.SetPaymentInterval(expQR.MaxPaymentInterval, expQR.MaxPaymentIntervalIncrease)
	provider2.SetPricePerByte(expQR.MinPricePerByte)
	require.NoError(t, provider2.Start())
	require.Equal(t, did, retrievalmarket.DealID(0))

	// verify that client subscribers will be notified of state changes
	ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
	defer cancel()
	var clientDealState retrievalmarket.ClientDealState
	select {
	case <-ctx.Done():
		t.Error("deal never completed")
		t.FailNow()
	case clientDealState = <-clientDealStateChan:
	}
	assert.Equal(t, clientDealState.PaymentInfo.Lane, ch.ExpectedVoucher.Lane)
	require.NotNil(t, ch.CreatedPayChInfo)
	require.Equal(t, expectedTotal, ch.CreatedPayChInfo.Amt)
	// verify that the voucher was saved/seen by the client with correct values
	require.NotNil(t, ch.CreatedVoucher)
	tut.TestVoucherEquality(t, ch.CreatedVoucher, ch.ExpectedVoucher)

	// verify that the provider saved the same voucher values
	ph.ProviderNode.VerifyExpectations(t)
	ch.TestData.VerifyFileTransferred(t, ph.PieceLink, false, filesize)
}

func TestStartStopClient(t *testing.T) {
	log.SetDebugLogging()
	filename := "lorem.txt"
	filesize := uint64(19000)
	voucherAmts := []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)}

	bgCtx := context.Background()
	ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, false)
	ph := new(test_harnesses.ProviderHarness)
	ph.TestData = ch.TestData
	ph.PaychAddr = ch.PaychAddr
	ph.Bootstrap(bgCtx, t, filename, filesize, false)

	expQR := ph.ExpectedQR
	retrievalPeer := &retrievalmarket.RetrievalPeer{
		Address: expQR.PaymentAddress, ID: ph.TestData.Host2.ID(),
	}
	expectedTotal := big.Mul(expQR.MinPricePerByte, abi.NewTokenAmount(int64(filesize*2)))

	proof := []byte("")
	for _, voucherAmt := range voucherAmts {
		require.NoError(t, ph.ProviderNode.ExpectVoucher(ph.PaychAddr, ch.ExpectedVoucher,
			proof, voucherAmt, voucherAmt, nil))
	}
	// **** Send the query for the Piece
	// set up retrieval params
	resp, err := ch.Query(bgCtx, *retrievalPeer, ph.PayloadCID, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	rmParams := retrievalmarket.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
		expQR.MaxPaymentIntervalIncrease, shared.AllSelector(), nil)
	// *** Retrieve the piece, simulating a shutdown in between
	_, err = ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
		ch.PaychAddr, retrievalPeer.Address)
	require.NoError(t, err)

	// Stop the client.
	require.NoError(t, ch.Stop())

	ch2, err := retrievalimpl.NewClient(ch.Network, ch.TestData.Bs1, ch.ClientNode, &tut.TestPeerResolver{},
		ch.TestData.Ds1, ch.TestData.RetrievalStoredCounter1)
	require.NoError(t, err)

	clientDealStateChan := make(chan retrievalmarket.ClientDealState)
	ch2.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
		switch event {
		case retrievalmarket.ClientEventComplete:
			clientDealStateChan <- state
		}
	})

	// verify that client subscribers will be notified of state changes
	ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
	defer cancel()
	var clientDealState retrievalmarket.ClientDealState
	select {
	case <-ctx.Done():
		t.Error("deal never completed")
		t.FailNow()
	case clientDealState = <-clientDealStateChan:
	}
	assert.Equal(t, clientDealState.PaymentInfo.Lane, ch.ExpectedVoucher.Lane)
	require.NotNil(t, ch.CreatedPayChInfo)
	require.Equal(t, expectedTotal, ch.CreatedPayChInfo.Amt)
	// verify that the voucher was saved/seen by the client with correct values
	require.NotNil(t, ch.CreatedVoucher)
	tut.TestVoucherEquality(t, ch.CreatedVoucher, ch.ExpectedVoucher)

	// verify that the provider saved the same voucher values
	ph.ProviderNode.VerifyExpectations(t)
	ch.TestData.VerifyFileTransferred(t, ph.PieceLink, false, filesize)
}

func logProviderDealState(t *testing.T, state retrievalmarket.ProviderDealState) {
	msg := `
Provider:
Status:          %s
TotalSent:       %d
FundsReceived:   %s
Message:		 %s
CurrentInterval: %d
`
	t.Logf(msg, retrievalmarket.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
		state.CurrentInterval)
}

func logClientDealState(t *testing.T, state retrievalmarket.ClientDealState) {
	msg := `
Client:
Status:          %s
TotalReceived:   %d
BytesPaidFor:    %d
CurrentInterval: %d
TotalFunds:      %s
Message:         %s
`
	t.Logf(msg, retrievalmarket.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
		state.TotalFunds.String(), state.Message)
}
