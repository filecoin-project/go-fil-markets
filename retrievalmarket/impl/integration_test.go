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
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/test_harnesses"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	client, expectedCIDs, missingPiece, expectedQR, retrievalPeer, _ := requireSetupTestClientAndProvider(bgCtx, t)

	t.Run("when piece is found, returns piece and price data", func(t *testing.T) {
		expectedQR.Status = rm.QueryResponseAvailable
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], rm.QueryParams{})

		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when piece is not found, returns unavailable", func(t *testing.T) {
		expectedQR.PieceCIDFound = rm.QueryItemUnavailable
		expectedQR.Status = rm.QueryResponseUnavailable
		expectedQR.Size = 0
		actualQR, err := client.Query(bgCtx, retrievalPeer, missingPiece, rm.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when there is some other error, returns error", func(t *testing.T) {
		unknownPiece := tut.GenerateCids(1)[0]
		expectedQR.Status = rm.QueryResponseError
		expectedQR.Message = "get cid info: GetCIDInfo failed"
		actualQR, err := client.Query(bgCtx, retrievalPeer, unknownPiece, rm.QueryParams{})
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})
}

func TestProvider_Stop(t *testing.T) {
	bgCtx := context.Background()
	client, expectedCIDs, _, _, retrievalPeer, provider := requireSetupTestClientAndProvider(bgCtx, t)
	require.NoError(t, provider.Stop())
	_, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], rm.QueryParams{})
	assert.EqualError(t, err, "protocol not supported")
}

func requireSetupTestClientAndProvider(bgCtx context.Context, t *testing.T) (rm.RetrievalClient,
	[]cid.Cid,
	cid.Cid,
	rm.QueryResponse,
	rm.RetrievalPeer,
	rm.RetrievalProvider) {

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

	retrievalPeer := rm.RetrievalPeer{
		Address: paymentAddress,
		ID:      ch.TestData.Host2.ID(),
	}
	return ch.RetrievalClient, expectedCIDs, missingCID, expectedQR, retrievalPeer, provider
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	log.SetDebugLogging()

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
		{name: "succeeds when using a custom provideropts function",
			decider: func(ctx context.Context, state rm.ProviderDealState) (bool, string, error) {
				customDeciderRan = true
				return true, "", nil
			},
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			unsealing:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bgCtx := context.Background()

			// ------- SET UP CLIENT
			ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, tc.addFunds)

			clientDealStateChan := make(chan rm.ClientDealState)
			ch.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
				switch event {
				case rm.ClientEventComplete:
					clientDealStateChan <- state
				default:
					logClientDealState(t, state)
				}
			})

			ph := new(test_harnesses.ProviderHarness)
			ph.TestData = ch.TestData
			ph.PaychAddr = ch.PaychAddr
			if tc.decider != nil {
				ph.ProviderOpts = []retrievalimpl.RetrievalProviderOption{
					retrievalimpl.DealDeciderOpt(tc.decider),
				}
			}
			ph.Bootstrap(bgCtx, t, tc.filename, tc.filesize, tc.unsealing)

			expQR := ph.ExpectedQR
			retrievalPeer := &rm.RetrievalPeer{
				Address: expQR.PaymentAddress, ID: ph.TestData.Host2.ID(),
			}

			// just make sure there is enough to cover the transfer
			expectedTotal := big.Mul(expQR.MinPricePerByte, abi.NewTokenAmount(int64(tc.filesize*2)))

			// voucherAmts are pulled from the actual answer so the expected keys in the test node match up.
			// later we compare the voucher values.  The last voucherAmt is a remainder
			proof := []byte("")
			for _, voucherAmt := range tc.voucherAmts {
				require.NoError(t, ph.ProviderNode.ExpectVoucher(ph.PaychAddr, ch.ExpectedVoucher, proof, voucherAmt, voucherAmt, nil))
			}

			providerDealStateChan := make(chan rm.ProviderDealState)
			ph.SubscribeToEvents(func(event rm.ProviderEvent, state rm.ProviderDealState) {
				switch event {
				case rm.ProviderEventComplete:
					providerDealStateChan <- state
				default:
					logProviderDealState(t, state)
				}
			})

			// **** Send the query for the Piece
			// set up retrieval params
			resp, err := ch.Query(bgCtx, *retrievalPeer, ph.PayloadCID, rm.QueryParams{})
			require.NoError(t, err)
			require.Equal(t, rm.QueryResponseAvailable, resp.Status)

			var rmParams rm.Params
			if tc.paramsV1 {
				rmParams = rm.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
					expQR.MaxPaymentIntervalIncrease, tc.selector, nil)

			} else {
				rmParams = rm.NewParamsV0(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
					expQR.MaxPaymentIntervalIncrease)
			}

			// *** Retrieve the piece, simulating a shutdown in between
			did, err := ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
				ph.PaychAddr, retrievalPeer.Address)
			require.NoError(t, err)

			require.Equal(t, did, rm.DealID(0))

			// verify that client subscribers will be notified of state changes
			ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			var clientDealState rm.ClientDealState
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
			var providerDealState rm.ProviderDealState
			select {
			case <-ctx.Done():
				t.Error("provider never saw completed deal")
				t.FailNow()
			case providerDealState = <-providerDealStateChan:
			}

			assert.Equal(t, int(rm.DealStatusCompleted), int(providerDealState.Status))
			// TODO: add asserts
			if tc.decider != nil {
				assert.True(t, customDeciderRan)
			}
			// verify that the provider saved the same voucher values
			ph.ProviderNode.VerifyExpectations(t)
			ph.TestData.VerifyFileTransferred(t, ph.PieceLink, false, tc.filesize)
		})
	}

}

/// =======================
func TestRestartProvider(t *testing.T) {
	bgCtx := context.Background()

	ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, false)
	ph := new(test_harnesses.ProviderHarness)
	ph.TestData = ch.TestData
	ph.PaychAddr = ch.PaychAddr
	filesize := 19000
	ph.Bootstrap(bgCtx, t, "lorem.txt", uint64(filesize), false)

	expQR, expectedTotal, retrievalPeer := setUpRetrievalParams(t, ph, ch, filesize)
	rmParams := rm.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
		expQR.MaxPaymentIntervalIncrease, shared.AllSelector(), nil)

	ch.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
		switch event {
		case rm.ClientEventDealAccepted:
			require.NoError(t, ph.Stop())
			require.NoError(t, ph.Start())
		}
	})

	stateChan := make(chan rm.ProviderDealState)
	ph.SubscribeToEvents(func(event rm.ProviderEvent, state rm.ProviderDealState) {
		switch event {
		case rm.ProviderEventDealRestart:
			stateChan <- state
		default:
			stateChan <- state
		}
	})

	go func() {
		_, err := ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
			ch.PaychAddr, retrievalPeer.Address)
		require.NoError(t, err)
	}()

	ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
	defer cancel()
	var seenState rm.ProviderDealState
	seen := 0
	// wait for client to process events past the restart.
	// it will not complete the transfer due to connection issues between
	// client and provider.
	for seen < 5 {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out, seen: %d", seen)
		case seenState = <-stateChan:
			logProviderDealState(t, seenState)
			seen++
		}
	}
	// checking only that provider continues processing
	// TODO: this isn't the correct ending status.
	assert.Equal(t, rm.DealStatusFundsNeeded, seenState.Status)
}

func TestRestartClient(t *testing.T) {
	bgCtx := context.Background()

	ch := new(test_harnesses.ClientHarness).Bootstrap(bgCtx, t, false)
	ph := new(test_harnesses.ProviderHarness)
	ph.TestData = ch.TestData
	ph.PaychAddr = ch.PaychAddr

	filesize := 19000
	ph.Bootstrap(bgCtx, t, "lorem.txt", uint64(filesize), false)

	expQR, expectedTotal, retrievalPeer := setUpRetrievalParams(t, ph, ch, filesize)

	rmParams := rm.NewParamsV1(expQR.MinPricePerByte, expQR.MaxPaymentInterval,
		expQR.MaxPaymentIntervalIncrease, shared.AllSelector(), nil)

	ph.SubscribeToEvents(func(event rm.ProviderEvent, state rm.ProviderDealState) {
		switch event {
		case rm.ProviderEventDealAccepted:
			require.NoError(t, ch.Run())
		}
	})

	evtChan := make(chan rm.ClientEvent)
	stateChan := make(chan rm.ClientDealState)
	ch.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
		switch event {
		case rm.ClientEventDealRestart:
			evtChan <- event
		case rm.ClientEventPaymentRequested:
			stateChan <- state
		}
	})

	_, err := ch.Retrieve(bgCtx, ph.PayloadCID, rmParams, expectedTotal, retrievalPeer.ID,
		ch.PaychAddr, retrievalPeer.Address)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(bgCtx, time.Second)
	defer cancel()
	var seenState rm.ClientDealState
	var seen int
	// wait for client to process past the restart
	for seen < 2 {
		select {
		case <-ctx.Done():
			t.Error("bad")
			t.FailNow()
		case seenState = <-stateChan:
			seen++
		case <-evtChan:
			seen++
		}
	}
	assert.Equal(t, rm.DealStatusFundsNeeded, seenState.Status)
}

func setUpRetrievalParams(t *testing.T, ph *test_harnesses.ProviderHarness, ch *test_harnesses.ClientHarness, filesize int) (
	*rm.QueryResponse, big.Int, *rm.RetrievalPeer) {
	expQR := ph.ExpectedQR
	retrievalPeer := &rm.RetrievalPeer{
		Address: expQR.PaymentAddress, ID: ph.TestData.Host2.ID(),
	}
	expectedTotal := big.Mul(expQR.MinPricePerByte, abi.NewTokenAmount(int64(filesize*2)))
	proof := []byte("")
	voucherAmts := []abi.TokenAmount{
		abi.NewTokenAmount(10136000),
		abi.NewTokenAmount(9784000),
	}
	for _, voucherAmt := range voucherAmts {
		require.NoError(t, ph.ProviderNode.ExpectVoucher(ph.PaychAddr, ch.ExpectedVoucher,
			proof, voucherAmt, voucherAmt, nil))
	}
	return expQR, expectedTotal, retrievalPeer
}

func logProviderDealState(t *testing.T, state rm.ProviderDealState) {
	msg := `
Provider:
Status:          %s
TotalSent:       %d
FundsReceived:   %s
Message:		 %s
CurrentInterval: %d
`
	t.Logf(msg, rm.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
		state.CurrentInterval)
}

func logClientDealState(t *testing.T, state rm.ClientDealState) {
	msg := `
Client:
Status:          %s
TotalReceived:   %d
BytesPaidFor:    %d
CurrentInterval: %d
TotalFunds:      %s
Message:         %s
`
	t.Logf(msg, rm.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
		state.TotalFunds.String(), state.Message)
}
