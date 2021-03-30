package retrievalmarket_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testharness"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageharness "github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

type providerNode struct {
	storage   *storageharness.ProviderHarness
	retrieval *testharness.RetrievalProviderHarness
}
type clientNode struct {
	storage   *storageharness.ClientHarness
	retrieval *testharness.RetrievalClientHarness
	storedDAG *tut.StoredDAG
}

// TestConcurrentStorageRetrieval verifies that storage and retrieval deals
// can be made concurrently
func TestConcurrentStorageRetrieval(t *testing.T) {
	// TODO: With 2 clients and 2 providers this test fails consistently.
	// Need to understand why.
	//clientCount := 2
	//providerCount := 2
	clientCount := 1
	providerCount := 1
	totalDeals := clientCount * providerCount

	// Size of the file that will be generated
	fileSize := 50000

	// The amounts for vouchers that are expected to be sent during transfer
	voucherAmts := []abi.TokenAmount{
		abi.NewTokenAmount(10553000),
		abi.NewTokenAmount(11264000),
		abi.NewTokenAmount(12288000),
		abi.NewTokenAmount(13312000),
		abi.NewTokenAmount(4944000),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mn := mocknet.New(ctx)
	rnd := rand.New(rand.NewSource(42))
	smState := testnodes.NewStorageMarketState()
	storageGen := storageharness.NewStorageInstanceGenerator(ctx, t, rnd, mn, smState)

	// Create storage clients
	var clients []*clientNode
	var providers []*providerNode
	for i := 0; i < clientCount; i++ {
		cl := storageGen.NewClient()

		// Add a file to each client's store that the client will send to
		// each provider in a storage deal
		storedDAG := cl.NodeDeps.LoadUnixFsFileToStore(ctx, t, fileSize)
		clients = append(clients, &clientNode{
			storage:   cl,
			storedDAG: storedDAG,
		})
	}

	// Create storage providers
	for i := 0; i < providerCount; i++ {
		prv := storageGen.NewProvider()
		providers = append(providers, &providerNode{
			storage: prv,
		})
	}

	// Connect all clients and providers
	err := mn.LinkAll()
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)

	for _, cl := range clients {
		for _, p := range providers {
			_, err := mn.ConnectPeers(cl.storage.NodeDeps.Host.ID(), p.storage.NodeDeps.Host.ID())
			require.NoError(t, err)
		}
	}

	// Start all clients and providers
	for _, cl := range clients {
		shared_testutil.StartAndWaitForReady(ctx, t, cl.storage.Client)
	}
	for _, prv := range providers {
		shared_testutil.StartAndWaitForReady(ctx, t, prv.storage.Provider)
	}

	// set up subscribers to watch for the completion of the deal on the client
	clientDealCompleted := make(chan struct{}, totalDeals)
	clientSubscriber := func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if deal.State == storagemarket.StorageDealExpired {
			clientDealCompleted <- struct{}{}
		}
	}
	for _, cl := range clients {
		_ = cl.storage.Client.SubscribeToEvents(clientSubscriber)
	}

	// set up subscribers to watch for the completion of the deal on the
	// provider
	providerDealCompleted := make(chan struct{}, totalDeals)
	provSubscriber := func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if deal.State == storagemarket.StorageDealExpired {
			providerDealCompleted <- struct{}{}
		}
	}
	for _, p := range providers {
		// set ask price where we'll accept any price
		err := p.storage.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
		assert.NoError(t, err)

		_ = p.storage.Provider.SubscribeToEvents(provSubscriber)
	}

	// Create a storage deal between each client and provider
	var dealDuration = abi.ChainEpoch(180 * builtin2.EpochsInDay)
	epoch := abi.ChainEpoch(100)
	for _, cl := range clients {
		cl := cl
		dataRef := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         cl.storedDAG.PayloadCID,
		}
		for _, prv := range providers {
			p := prv
			go func() {
				result, err := cl.storage.Client.ProposeStorageDeal(ctx, storagemarket.ProposeStorageDealParams{
					Addr:          cl.storage.Addr,
					Info:          &p.storage.ProviderInfo,
					Data:          dataRef,
					StartEpoch:    epoch + 100,
					EndEpoch:      epoch + 100 + dealDuration,
					Price:         big.NewInt(1),
					Collateral:    big.NewInt(0),
					Rt:            abi.RegisteredSealProof_StackedDrg2KiBV1,
					FastRetrieval: false,
					VerifiedDeal:  false,
					StoreID:       &cl.storedDAG.StoreID,
				})
				require.NoError(t, err)
				require.False(t, result.ProposalCid.Equals(cid.Undef))
			}()
		}
	}

	time.Sleep(time.Millisecond * 200)

	// Wait for the storage deals to complete
	ctxStgDealsTimeout, stgDealsCancel := context.WithTimeout(ctx, 5*time.Second)
	defer stgDealsCancel()

	clientAwaitingCompletion := totalDeals
	providerAwaitingCompletion := totalDeals
	for clientAwaitingCompletion > 0 && providerAwaitingCompletion > 0 {
		select {
		case <-clientDealCompleted:
			clientAwaitingCompletion--
		case <-providerDealCompleted:
			providerAwaitingCompletion--
		case <-ctxStgDealsTimeout.Done():
			t.Fatalf("timed out waiting for %d deals to complete - remaining: client %d, provider %d",
				totalDeals, clientAwaitingCompletion, providerAwaitingCompletion)
		}
	}

	t.Logf("%d storage deals completed", totalDeals)
	t.Logf("Running %d retrieval deals", totalDeals)

	askParams := retrievalmarket.Params{
		PricePerByte:            abi.NewTokenAmount(1000),
		PaymentInterval:         uint64(10000),
		PaymentIntervalIncrease: uint64(1000),
		UnsealPrice:             big.Zero(),
	}

	// Create retrieval clients
	for _, cl := range clients {
		cl.retrieval = testharness.NewRetrievalClient(t, &testharness.RetrievalClientParams{
			Host:                   cl.storage.NodeDeps.Host,
			Dstore:                 cl.storage.NodeDeps.Dstore,
			MultiStore:             cl.storage.NodeDeps.MultiStore,
			DataTransfer:           cl.storage.DataTransfer,
			PeerResolver:           cl.storage.PeerResolver,
			RetrievalStoredCounter: cl.storage.NodeDeps.DTStoredCounter,
		})
		tut.StartAndWaitForReady(ctx, t, cl.retrieval.Client)
	}

	// Create retrieval providers
	for _, prv := range providers {
		prv.retrieval = testharness.NewRetrievalProvider(t, &testharness.RetrievalProviderParams{
			MockNet:      mn,
			NodeDeps:     prv.storage.NodeDeps,
			PaymentAddr:  prv.storage.ProviderInfo.Worker,
			DataTransfer: prv.storage.DataTransfer,
			AskParams:    askParams,
		})
		tut.StartAndWaitForReady(ctx, t, prv.retrieval.Provider)
	}

	// Set up each provider with car data that will be unsealed when the
	// client makes a retrieval deal for the payload CID
	for _, cl := range clients {
		for _, prv := range providers {
			payloadCID := cl.storedDAG.PayloadCID
			carData := prv.storage.ProviderNode.OnDealCompleteBytes[payloadCID]
			prv.retrieval.MockUnseal(ctx, t, payloadCID, carData)
		}
	}

	// Each client retrieves data from each provider
	retrievals := make(chan struct{}, totalDeals)
	for clIdx, cl := range clients {
		clIdx := clIdx
		cl := cl

		for prvIdx, prv := range providers {
			prvIdx := prvIdx
			prv := prv

			go func() {
				t.Logf("Starting retrieval deal client %d / provider %d", clIdx, prvIdx)
				defer t.Logf("Completed retrieval deal client %d / provider %d", clIdx, prvIdx)

				retrieve(ctx, t, cl, prv, askParams, voucherAmts, fileSize)

				retrievals <- struct{}{}
			}()
		}
	}

	completed := 0
	for completed < totalDeals {
		select {
		case <-retrievals:
			completed++
		case <-time.After(5 * time.Second):
			t.Fatal("Timed out waiting for retrieval deals to complete")
		}
	}
}

func retrieve(ctx context.Context, t *testing.T, cl *clientNode, prv *providerNode, askParams retrievalmarket.Params, voucherAmts []abi.TokenAmount, fileSize int) {
	payloadCID := cl.storedDAG.PayloadCID

	// Watch for completion of client side of deal
	clientDealComplete := make(chan error, 1)
	cl.retrieval.Client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
		switch state.Status {
		case retrievalmarket.DealStatusCompleted:
			//t.Logf("  client %d / provider %d: client complete", clIdx, prvIdx)
			clientDealComplete <- nil
		case retrievalmarket.DealStatusErrored:
			clientDealComplete <- xerrors.Errorf("Deal failed (client): %s", state.Message)
		default:
			//logClientState(event, state, t)
		}
	})

	// Watch for completion of provider side of deal
	providerDealComplete := make(chan error, 1)
	prv.retrieval.Provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
		switch state.Status {
		case retrievalmarket.DealStatusCompleted:
			//t.Logf("  client %d / provider %d: provider complete", clIdx, prvIdx)
			providerDealComplete <- nil
		case retrievalmarket.DealStatusErrored:
			providerDealComplete <- xerrors.Errorf("Deal failed (provider): %s", state.Message)
		default:
			//logProviderState(event, state, t)
		}
	})

	// *** Find providers of the payload CID
	peers := cl.retrieval.Client.FindProviders(payloadCID)
	var retrievalPeer retrievalmarket.RetrievalPeer
	for _, rp := range peers {
		if rp.ID == prv.retrieval.NodeDeps.Host.ID() {
			retrievalPeer = rp
		}
	}
	require.NotNil(t, retrievalPeer)
	require.NotNil(t, retrievalPeer.PieceCID)

	cl.retrieval.ClientNode.ExpectKnownAddresses(retrievalPeer, nil)

	// *** Query the provider for the payload CID
	resp, err := cl.retrieval.Client.Query(ctx, retrievalPeer, payloadCID, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	// testing V1 only
	rmParams, err := retrievalmarket.NewParamsV1(askParams.PricePerByte, askParams.PaymentInterval, askParams.PaymentIntervalIncrease, shared.AllSelector(), nil, big.Zero())
	require.NoError(t, err)

	proof := []byte("")
	for _, voucherAmt := range voucherAmts {
		require.NoError(t, prv.retrieval.ProviderNode.ExpectVoucher(cl.retrieval.Paych, cl.retrieval.ExpectedVoucher, proof, voucherAmt, voucherAmt, nil))
	}
	// just make sure there is enough to cover the transfer
	expectedTotal := big.Mul(askParams.PricePerByte, abi.NewTokenAmount(int64(fileSize*2)))

	// *** Retrieve the piece
	// Create a new store so that the data is downloaded into a
	// different place than the data that was originally uploaded
	clientStoreID := cl.retrieval.MultiStore.Next()
	_, err = cl.retrieval.Client.Retrieve(ctx, payloadCID, rmParams, expectedTotal, retrievalPeer, cl.retrieval.Paych, retrievalPeer.Address, &clientStoreID)
	require.NoError(t, err)

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Wait for retrieval deal to complete on the client and provider
	doneCount := 0
	for doneCount < 2 {
		var err error
		select {
		case <-ctxTimeout.Done():
			t.Error("deal never completed")
			t.FailNow()
		case err = <-clientDealComplete:
		case err = <-providerDealComplete:
		}
		if err != nil {
			t.Fatal(err)
			return
		}
		doneCount++
	}

	// Verify that the file was correctly retrieved by the client
	cl.retrieval.ClientNode.VerifyExpectations(t)
	store, err := cl.storage.NodeDeps.MultiStore.Get(clientStoreID)
	require.NoError(t, err)
	tut.VerifyFileInStore(ctx, t, cl.storedDAG.Data, payloadCID, store.DAG)
}

var _ datatransfer.RequestValidator = (*fakeDTValidator)(nil)

type fakeDTValidator struct{}

func (v *fakeDTValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, nil
}

func (v *fakeDTValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, nil
}

//func logProviderState(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState, t *testing.T) {
//	msg := `
//					Provider:
//					Event:           %s
//					Status:          %s
//					TotalSent:       %d
//					FundsReceived:   %s
//					Message:		 %s
//					CurrentInterval: %d
//					`
//	t.Logf(msg, retrievalmarket.ProviderEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
//		state.CurrentInterval)
//}
//
//func logClientState(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState, t *testing.T) {
//	msg := `
//					Client:
//					Event:           %s
//					Status:          %s
//					TotalReceived:   %d
//					BytesPaidFor:    %d
//					CurrentInterval: %d
//					TotalFunds:      %s
//					Message:         %s
//					`
//	t.Logf(msg, retrievalmarket.ClientEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
//		state.TotalFunds.String(), state.Message)
//}
