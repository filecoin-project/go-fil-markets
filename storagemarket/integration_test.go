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

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestMakeDeal(t *testing.T) {
	ctx := context.Background()
	testCases := map[string]bool{
		"with stores":          true,
		"with just blockstore": false,
	}
	for testCase, useStore := range testCases {
		t.Run(testCase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, useStore, testnodes.DelayFakeCommonNode{})
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
				storagemarket.StorageDealEnsureProviderFunds,
				storagemarket.StorageDealPublish,
				storagemarket.StorageDealPublishing,
				storagemarket.StorageDealStaged,
				storagemarket.StorageDealSealing,
				storagemarket.StorageDealFinalizing,
				storagemarket.StorageDealActive,
				storagemarket.StorageDealExpired,
			}

			expClientStates := []storagemarket.StorageDealStatus{
				storagemarket.StorageDealEnsureClientFunds,
				//storagemarket.StorageDealClientFunding,  // skipped because funds available
				storagemarket.StorageDealFundsEnsured,
				storagemarket.StorageDealStartDataTransfer,
				storagemarket.StorageDealTransferring,
				storagemarket.StorageDealCheckForAcceptance,
				storagemarket.StorageDealProposalAccepted,
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
	h := testharness.NewHarness(t, ctx, true, testnodes.DelayFakeCommonNode{})
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	carBuf := new(bytes.Buffer)

	store, err := h.TestData.MultiStore1.Get(*h.StoreID)
	require.NoError(t, err)

	err = cario.NewCarIO().WriteCar(ctx, store.Bstore, h.PayloadCid, shared.AllSelector(), carBuf)
	require.NoError(t, err)

	commP, size, err := pieceio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, carBuf, uint64(carBuf.Len()))
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
	wg.Wait()

	cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	shared_testutil.AssertDealState(t, storagemarket.StorageDealWaitingForData, pd.State)

	err = cario.NewCarIO().WriteCar(ctx, store.Bstore, h.PayloadCid, shared.AllSelector(), carBuf)
	require.NoError(t, err)
	err = h.Provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
	require.NoError(t, err)

	h.WaitForClientEvent(&wg, storagemarket.ClientEventDealExpired)
	h.WaitForProviderEvent(&wg, storagemarket.ProviderEventDealExpired)
	wg.Wait()

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
	h := testharness.NewHarness(t, ctx, true, testnodes.DelayFakeCommonNode{})
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
	wg.Wait()

	cd, err := h.Client.GetLocalDeal(ctx, result.ProposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	// Provider should be blocking on waiting for funds to appear on chain
	pd := providerDeals[0]
	assert.Equal(t, result.ProposalCid, pd.ProposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealProviderFunding, pd.State)
}

func TestRestartClient(t *testing.T) {
	testCases := map[string]struct {
		stopAtEvent storagemarket.ClientEvent
		fh          func(h *testharness.StorageHarness)
	}{
		"ClientEventFundsEnsured": {
			stopAtEvent: storagemarket.ClientEventFundsEnsured,
		},
		"ClientEventInitiateDataTransfer": {
			stopAtEvent: storagemarket.ClientEventInitiateDataTransfer,
		},
		"ClientEventDataTransferInitiated": {
			stopAtEvent: storagemarket.ClientEventDataTransferInitiated,
		},
		"ClientEventDataTransferComplete": {
			stopAtEvent: storagemarket.ClientEventDataTransferComplete,
		},
		"ClientEventDealAccepted": {
			stopAtEvent: storagemarket.ClientEventDealAccepted,
		},
		"ClientEventDealActivated": {
			stopAtEvent: storagemarket.ClientEventDealActivated,
			fh: func(h *testharness.StorageHarness) {
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
			},
		},
		"ClientEventDealPublished": {
			stopAtEvent: storagemarket.ClientEventDealPublished,
			fh: func(h *testharness.StorageHarness) {
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, true, testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true,
				OnDealSectorCommitted: true})

			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
			assert.NoError(t, err)

			wg := sync.WaitGroup{}
			wg.Add(1)
			var providerState []storagemarket.MinerDeal
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == tc.stopAtEvent {
					// Stop the client and provider at some point during deal negotiation
					require.NoError(t, h.Client.Stop())
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

			if tc.fh != nil {
				tc.fh(h)
			}

			wg.Wait()

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			if tc.stopAtEvent != storagemarket.ClientEventDealActivated && tc.stopAtEvent != storagemarket.ClientEventDealPublished {
				assert.NotEqual(t, storagemarket.StorageDealActive, cd.State)
			}
			h = testharness.NewHarnessWithTestData(t, ctx, h.TestData, h.SMState, true, h.TempFilePath, testnodes.DelayFakeCommonNode{})

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

			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

			wg.Wait()

			cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

			providerDeals, err := h.Provider.ListLocalDeals()
			assert.NoError(t, err)

			pd := providerDeals[0]
			assert.Equal(t, pd.ProposalCid, proposalCid)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
		})
	}
}
