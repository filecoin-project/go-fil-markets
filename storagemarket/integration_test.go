package storagemarket_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-commp-utils/pieceio"
	"github.com/filecoin-project/go-commp-utils/pieceio/cario"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

func TestMakeDeal(t *testing.T) {
	ctx := context.Background()
	testCases := map[string]struct {
		useStore        bool
		disableNewDeals bool
	}{
		"with stores": {
			useStore: true,
		},
		"with just blockstore": {
			useStore: false,
		},
		"disable new protocols": {
			useStore:        true,
			disableNewDeals: true,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, data.useStore, noOpDelay, noOpDelay, data.disableNewDeals)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

			// set up a subscriber
			providerDealChan := make(chan storagemarket.MinerDeal)
			var checkedUnmarshalling bool
			subscriber := func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
				if !checkedUnmarshalling {
					// test that deal created can marshall and unmarshalled
					jsonBytes, err := json.Marshal(deal)
					require.NoError(t, err)
					var unmDeal storagemarket.MinerDeal
					err = json.Unmarshal(jsonBytes, &unmDeal)
					require.NoError(t, err)
					checkedUnmarshalling = true
				}
				providerDealChan <- deal
			}
			_ = h.Provider.SubscribeToEvents(subscriber)

			clientDealChan := make(chan storagemarket.ClientDeal)
			clientSubscriber := func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				clientDealChan <- deal
			}
			_ = h.Client.SubscribeToEvents(clientSubscriber)

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
			assert.NoError(t, err)

			result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, true, false)
			proposalCid := result.ProposalCid

			dealStatesToStrings := func(states []storagemarket.StorageDealStatus) []string {
				var out []string
				for _, state := range states {
					out = append(out, storagemarket.DealStates[state])
				}
				return out
			}

			var providerSeenDeal storagemarket.MinerDeal
			var clientSeenDeal storagemarket.ClientDeal
			var providerstates, clientstates []storagemarket.StorageDealStatus
			for providerSeenDeal.State != storagemarket.StorageDealExpired ||
				clientSeenDeal.State != storagemarket.StorageDealExpired {
				select {
				case <-ctx.Done():
					t.Fatalf(`did not see all states before context closed
			saw client: %v,
			saw provider: %v`, dealStatesToStrings(clientstates), dealStatesToStrings(providerstates))
				case clientSeenDeal = <-clientDealChan:
					if len(clientstates) == 0 || clientSeenDeal.State != clientstates[len(clientstates)-1] {
						clientstates = append(clientstates, clientSeenDeal.State)
					}
				case providerSeenDeal = <-providerDealChan:
					if len(providerstates) == 0 || providerSeenDeal.State != providerstates[len(providerstates)-1] {
						providerstates = append(providerstates, providerSeenDeal.State)
					}
				}
			}

			expProviderStates := []storagemarket.StorageDealStatus{
				storagemarket.StorageDealValidating,
				storagemarket.StorageDealAcceptWait,
				storagemarket.StorageDealWaitingForData,
				storagemarket.StorageDealTransferring,
				storagemarket.StorageDealVerifyData,
				storagemarket.StorageDealReserveProviderFunds,
				storagemarket.StorageDealPublish,
				storagemarket.StorageDealPublishing,
				storagemarket.StorageDealStaged,
				storagemarket.StorageDealAwaitingPreCommit,
				storagemarket.StorageDealSealing,
				storagemarket.StorageDealFinalizing,
				storagemarket.StorageDealActive,
				storagemarket.StorageDealExpired,
			}

			expClientStates := []storagemarket.StorageDealStatus{
				storagemarket.StorageDealReserveClientFunds,
				//storagemarket.StorageDealClientFunding,  // skipped because funds available
				storagemarket.StorageDealFundsReserved,
				storagemarket.StorageDealStartDataTransfer,
				storagemarket.StorageDealTransferring,
				storagemarket.StorageDealCheckForAcceptance,
				storagemarket.StorageDealProposalAccepted,
				storagemarket.StorageDealAwaitingPreCommit,
				storagemarket.StorageDealSealing,
				storagemarket.StorageDealActive,
				storagemarket.StorageDealExpired,
			}

			assert.Equal(t, dealStatesToStrings(expProviderStates), dealStatesToStrings(providerstates))
			assert.Equal(t, dealStatesToStrings(expClientStates), dealStatesToStrings(clientstates))

			// check a couple of things to make sure we're getting the whole deal
			assert.Equal(t, h.TestData.Host1.ID(), providerSeenDeal.Client)
			assert.Empty(t, providerSeenDeal.Message)
			assert.Equal(t, proposalCid, providerSeenDeal.ProposalCid)
			assert.Equal(t, h.ProviderAddr, providerSeenDeal.ClientDealProposal.Proposal.Provider)

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)
			assert.True(t, cd.FastRetrieval)

			providerDeals, err := h.Provider.ListLocalDeals()
			assert.NoError(t, err)

			pd := providerDeals[0]
			assert.Equal(t, proposalCid, pd.ProposalCid)
			assert.True(t, pd.FastRetrieval)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)

			// test out query protocol
			status, err := h.Client.GetProviderDealState(ctx, proposalCid)
			assert.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, status.State)
			assert.True(t, status.FastRetrieval)

			// ensure that the handoff has fast retrieval info
			assert.Len(t, h.ProviderNode.OnDealCompleteCalls, 1)
			assert.True(t, h.ProviderNode.OnDealCompleteCalls[0].FastRetrieval)
			h.ClientNode.VerifyExpectations(t)
		})
	}
}

func TestMakeDealOffline(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	store, err := h.TestData.MultiStore1.Get(*h.StoreID)
	require.NoError(t, err)

	cio := cario.NewCarIO()
	pio := pieceio.NewPieceIO(cio, store.Bstore, h.TestData.MultiStore1)

	commP, size, err := pio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, h.PayloadCid, shared.AllSelector(), h.StoreID)
	assert.NoError(t, err)

	dataRef := &storagemarket.DataRef{
		TransferType: storagemarket.TTManual,
		Root:         h.PayloadCid,
		PieceCid:     &commP,
		PieceSize:    size,
	}

	result := h.ProposeStorageDeal(t, dataRef, false, false)
	proposalCid := result.ProposalCid

	wg := sync.WaitGroup{}

	h.WaitForClientEvent(&wg, storagemarket.ClientEventDataTransferComplete)
	h.WaitForProviderEvent(&wg, storagemarket.ProviderEventDataRequested)
	waitGroupWait(ctx, &wg)

	cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	require.Eventually(t, func() bool {
		cd, _ = h.Client.GetLocalDeal(ctx, proposalCid)
		return cd.State == storagemarket.StorageDealCheckForAcceptance
	}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[cd.State])

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	shared_testutil.AssertDealState(t, storagemarket.StorageDealWaitingForData, pd.State)

	carBuf := new(bytes.Buffer)
	err = cio.WriteCar(ctx, store.Bstore, h.PayloadCid, shared.AllSelector(), carBuf)
	require.NoError(t, err)
	require.NoError(t, err)
	err = h.Provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
	require.NoError(t, err)

	h.WaitForClientEvent(&wg, storagemarket.ClientEventDealExpired)
	h.WaitForProviderEvent(&wg, storagemarket.ProviderEventDealExpired)
	waitGroupWait(ctx, &wg)

	cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

	providerDeals, err = h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd = providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
}

func TestMakeDealNonBlocking(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)

	testCids := shared_testutil.GenerateCids(2)

	h.ProviderNode.WaitForMessageBlocks = true
	h.ProviderNode.AddFundsCid = testCids[1]
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)

	h.ClientNode.AddFundsCid = testCids[0]
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)

	wg := sync.WaitGroup{}
	h.WaitForClientEvent(&wg, storagemarket.ClientEventDataTransferComplete)
	h.WaitForProviderEvent(&wg, storagemarket.ProviderEventFundingInitiated)
	waitGroupWait(ctx, &wg)

	cd, err := h.Client.GetLocalDeal(ctx, result.ProposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	// Provider should be blocking on waiting for funds to appear on chain
	pd := providerDeals[0]
	assert.Equal(t, result.ProposalCid, pd.ProposalCid)
	require.Eventually(t, func() bool {
		providerDeals, err := h.Provider.ListLocalDeals()
		assert.NoError(t, err)
		pd = providerDeals[0]
		return pd.State == storagemarket.StorageDealProviderFunding
	}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[pd.State])
}

func TestRestartOnlyProviderDataTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)
	client := h.Client
	host1 := h.TestData.Host1
	host2 := h.TestData.Host2

	// start client and provider
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
	require.NoError(t, err)

	// wait for provider to enter deal transferring state and stop
	wg := sync.WaitGroup{}
	wg.Add(1)
	var providerState []storagemarket.MinerDeal
	_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == storagemarket.ProviderEventDataTransferInitiated {
			ev := storagemarket.ProviderEvents[event]
			t.Logf("event %s has happened on provider, shutting down provider", ev)
			require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.Provider.Stop())

			// deal could have expired already on the provider side for the `ClientEventDealAccepted` event
			// so, we should wait on the `ProviderEventDealExpired` event ONLY if the deal has not expired.
			providerState, err = h.Provider.ListLocalDeals()
			assert.NoError(t, err)
			wg.Done()
		}
	})

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
	proposalCid := result.ProposalCid
	t.Log("storage deal proposed")

	waitGroupWait(ctx, &wg)
	t.Log("provider has been shutdown the first time")

	// Assert client state
	cd, err := client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
	require.True(t, cd.State == storagemarket.StorageDealStartDataTransfer || cd.State == storagemarket.StorageDealTransferring)

	// RESTART ONLY PROVIDER
	h.CreateNewProvider(t, ctx, h.TestData, h.TempFilePath, false)

	t.Logf("provider state after stopping is %s", storagemarket.DealStates[providerState[0].State])
	require.Equal(t, storagemarket.StorageDealTransferring, providerState[0].State)

	expireWg := sync.WaitGroup{}
	expireWg.Add(1)
	_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == storagemarket.ProviderEventDealExpired {
			expireWg.Done()
		}
	})

	expireWg.Add(1)
	_ = client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventDealExpired {
			expireWg.Done()
		}
	})

	// sleep so go-data-transfer gives up on retries after creating new connection
	time.Sleep(15 * time.Second)
	t.Log("finished sleeping")
	require.NoError(t, h.TestData.MockNet.LinkAll())
	time.Sleep(200 * time.Millisecond)
	conn, err := h.TestData.MockNet.ConnectPeers(host1.ID(), host2.ID())
	require.NoError(t, err)
	require.NotNil(t, conn)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	t.Log("------- provider has been restarted---------")
	waitGroupWait(ctx, &expireWg)
	t.Log("---------- finished waiting for expected events-------")

	cd, err = client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	require.NoError(t, err)

	pd := providerDeals[0]
	require.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
}

// FIXME Gets hung sometimes
func TestRestartClient(t *testing.T) {
	testCases := map[string]struct {
		stopAtClientEvent   storagemarket.ClientEvent
		stopAtProviderEvent storagemarket.ProviderEvent

		expectedClientState storagemarket.StorageDealStatus
		clientDelay         testnodes.DelayFakeCommonNode
		providerDelay       testnodes.DelayFakeCommonNode
	}{

		"ClientEventDataTransferInitiated": {
			// This test can fail if client crashes without seeing a Provider DT complete
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtClientEvent:   storagemarket.ClientEventDataTransferInitiated,
			expectedClientState: storagemarket.StorageDealTransferring,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDataTransferComplete": {
			stopAtClientEvent:   storagemarket.ClientEventDataTransferComplete,
			stopAtProviderEvent: storagemarket.ProviderEventDataTransferCompleted,
			expectedClientState: storagemarket.StorageDealCheckForAcceptance,
		},

		"ClientEventFundingComplete": {
			//Edge case : Provider begins the state machine on recieving a deal stream request
			//client crashes -> restarts -> sends deal stream again -> state machine fails
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtClientEvent:   storagemarket.ClientEventFundingComplete,
			expectedClientState: storagemarket.StorageDealFundsReserved,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		// FIXME
		"ClientEventInitiateDataTransfer": { // works well but sometimes state progresses beyond StorageDealStartDataTransfer
			stopAtClientEvent:   storagemarket.ClientEventInitiateDataTransfer,
			expectedClientState: storagemarket.StorageDealStartDataTransfer,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDealAccepted": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealAccepted,
			expectedClientState: storagemarket.StorageDealProposalAccepted,
			clientDelay:         testnodes.DelayFakeCommonNode{ValidatePublishedDeal: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealActivated": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealActivated,
			expectedClientState: storagemarket.StorageDealActive,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealPublished": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealPublished,
			expectedClientState: storagemarket.StorageDealSealing,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealSectorCommitted: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, true, tc.clientDelay, tc.providerDelay, false)
			host1 := h.TestData.Host1
			host2 := h.TestData.Host2

			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
			require.NoError(t, err)

			wg := sync.WaitGroup{}
			wg.Add(1)
			var providerState []storagemarket.MinerDeal
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == tc.stopAtClientEvent {
					// Stop the client and provider at some point during deal negotiation
					ev := storagemarket.ClientEvents[event]
					t.Logf("event %s has happened on client, shutting down client and provider", ev)
					require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.Client.Stop())

					// if a provider stop event isn't specified, just stop the provider here
					if tc.stopAtProviderEvent == 0 {
						require.NoError(t, h.Provider.Stop())
					}

					// deal could have expired already on the provider side for the `ClientEventDealAccepted` event
					// so, we should wait on the `ProviderEventDealExpired` event ONLY if the deal has not expired.
					providerState, err = h.Provider.ListLocalDeals()
					assert.NoError(t, err)
					wg.Done()
				}
			})

			// if this test case specifies a provider stop event...
			if tc.stopAtProviderEvent != 0 {
				wg.Add(1)

				_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if event == tc.stopAtProviderEvent {
						require.NoError(t, h.Provider.Stop())
						wg.Done()
					}
				})
			}

			result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
			proposalCid := result.ProposalCid
			t.Log("storage deal proposed")

			waitGroupWait(ctx, &wg)
			t.Log("both client and provider have been shutdown the first time")

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			require.NoError(t, err)
			t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
			require.Equal(t, tc.expectedClientState, cd.State)

			h = testharness.NewHarnessWithTestData(t, ctx, h.TestData, h.SMState, true, h.TempFilePath, noOpDelay, noOpDelay,
				false)

			if len(providerState) == 0 {
				t.Log("no deal created on provider after stopping")
			} else {
				t.Logf("provider state after stopping is %s", storagemarket.DealStates[providerState[0].State])
			}

			if len(providerState) == 0 || providerState[0].State != storagemarket.StorageDealExpired {
				wg.Add(1)
				_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if event == storagemarket.ProviderEventDealExpired {
						wg.Done()
					}
				})
			}
			wg.Add(1)
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == storagemarket.ClientEventDealExpired {
					wg.Done()
				}
			})

			require.NoError(t, h.TestData.MockNet.LinkAll())
			time.Sleep(200 * time.Millisecond)
			conn, err := h.TestData.MockNet.ConnectPeers(host1.ID(), host2.ID())
			require.NoError(t, err)
			require.NotNil(t, conn)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)
			t.Log("------- client and provider have been restarted---------")
			waitGroupWait(ctx, &wg)
			t.Log("---------- finished waiting for expected events-------")

			cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
			require.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

			providerDeals, err := h.Provider.ListLocalDeals()
			require.NoError(t, err)

			pd := providerDeals[0]
			require.Equal(t, pd.ProposalCid, proposalCid)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
		})
	}
}

// waitGroupWait calls wg.Wait while respecting context cancellation
func waitGroupWait(ctx context.Context, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}
