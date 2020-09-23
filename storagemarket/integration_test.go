package storagemarket_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnetwork "github.com/ipfs/go-graphsync/network"
	ipld "github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

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
			h := newHarness(t, ctx, useStore, noOpDelay, noOpDelay)
			require.NoError(t, h.Provider.Start(ctx))
			require.NoError(t, h.Client.Start(ctx))

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
	h := newHarness(t, ctx, true, noOpDelay, noOpDelay)
	require.NoError(t, h.Client.Start(ctx))
	require.NoError(t, h.Provider.Start(ctx))

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
	h := newHarness(t, ctx, true, noOpDelay, noOpDelay)
	testCids := shared_testutil.GenerateCids(2)

	h.ProviderNode.WaitForMessageBlocks = true
	h.ProviderNode.AddFundsCid = testCids[1]
	require.NoError(t, h.Provider.Start(ctx))

	h.ClientNode.AddFundsCid = testCids[0]
	require.NoError(t, h.Client.Start(ctx))

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
	require.Eventually(t, func() bool {
		return pd.State == storagemarket.StorageDealProviderFunding
	}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[pd.State])
}

func TestRestartOnlyProviderDataTransfer(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	h := newHarness(t, ctx, true, noOpDelay, noOpDelay)
	client := h.Client
	host1 := h.TestData.Host1
	host2 := h.TestData.Host2

	// start client and provider
	require.NoError(t, h.Provider.Start(ctx))
	require.NoError(t, client.Start(ctx))

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
	require.NoError(t, err)

	// wait for provider to enter deal transferring state and stop
	wg := sync.WaitGroup{}
	wg.Add(1)
	_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == storagemarket.ProviderEventDataTransferInitiated {
			ev := storagemarket.ProviderEvents[event]
			t.Logf("event %s has happened on provider, shutting down provider", ev)
			require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.Provider.Stop())
			wg.Done()
		}
	})

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
	proposalCid := result.ProposalCid
	t.Log("storage deal proposed")

	wg.Wait()
	t.Log("provider has been shutdown the first time")

	// Assert client state
	cd, err := client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
	require.True(t, cd.State == storagemarket.StorageDealStartDataTransfer || cd.State == storagemarket.StorageDealTransferring)

	// RESTART ONLY PROVIDER
	h.createNewProvider(t, ctx, h.TestData, h.TempFilePath)
	pds, err := h.Provider.ListLocalDeals()
	require.NoError(t, err)
	t.Logf("provider state after stopping is %s", storagemarket.DealStates[pds[0].State])
	require.Equal(t, storagemarket.StorageDealTransferring, pds[0].State)

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
	require.NoError(t, h.Provider.Start(ctx))
	t.Log("------- provider has been restarted---------")
	expireWg.Wait()
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
		stopAtEvent         storagemarket.ClientEvent
		expectedClientState storagemarket.StorageDealStatus
		clientDelay         testnodes.DelayFakeCommonNode
		providerDelay       testnodes.DelayFakeCommonNode
		fh                  func(h *harness)
	}{

		"ClientEventDataTransferInitiated": {
			// This test can fail if client crashes without seeing a Provider DT complete
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtEvent:         storagemarket.ClientEventDataTransferInitiated,
			expectedClientState: storagemarket.StorageDealTransferring,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDataTransferComplete": {
			stopAtEvent:         storagemarket.ClientEventDataTransferComplete,
			expectedClientState: storagemarket.StorageDealCheckForAcceptance,
		},

		"ClientEventFundsEnsured": {
			//Edge case : Provider begins the state machine on recieving a deal stream request
			//client crashes -> restarts -> sends deal stream again -> state machine fails
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtEvent:         storagemarket.ClientEventFundsEnsured,
			expectedClientState: storagemarket.StorageDealFundsEnsured,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		// FIXME
		"ClientEventInitiateDataTransfer": { // works well but sometimes state progresses beyond StorageDealStartDataTransfer
			stopAtEvent:         storagemarket.ClientEventInitiateDataTransfer,
			expectedClientState: storagemarket.StorageDealStartDataTransfer,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDealAccepted": { // works well
			stopAtEvent:         storagemarket.ClientEventDealAccepted,
			expectedClientState: storagemarket.StorageDealProposalAccepted,
			clientDelay:         testnodes.DelayFakeCommonNode{ValidatePublishedDeal: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealActivated": { // works well
			stopAtEvent:         storagemarket.ClientEventDealActivated,
			expectedClientState: storagemarket.StorageDealActive,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealPublished": { // works well
			stopAtEvent:         storagemarket.ClientEventDealPublished,
			expectedClientState: storagemarket.StorageDealSealing,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealSectorCommitted: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			h := newHarness(t, ctx, true, tc.clientDelay, tc.providerDelay)
			host1 := h.TestData.Host1
			host2 := h.TestData.Host2

			require.NoError(t, h.Provider.Start(ctx))
			require.NoError(t, h.Client.Start(ctx))

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
			require.NoError(t, err)

			wg := sync.WaitGroup{}
			wg.Add(1)
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == tc.stopAtEvent {
					// Stop the client and provider at some point during deal negotiation
					ev := storagemarket.ClientEvents[event]
					t.Logf("event %s has happened on client, shutting down client and provider", ev)
					require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.Client.Stop())
					require.NoError(t, h.Provider.Stop())
					wg.Done()
				}
			})

			result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
			proposalCid := result.ProposalCid
			t.Log("storage deal proposed")
			if tc.fh != nil {
				tc.fh(h)
			}
			t.Log("node harness executed")

			wg.Wait()
			t.Log("both client and provider have been shutdown the first time")

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			require.NoError(t, err)
			t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
			require.Equal(t, tc.expectedClientState, cd.State)

			h = newHarnessWithTestData(t, ctx, h.TestData, h.SMState, true, h.TempFilePath, noOpDelay, noOpDelay)

			pds, err := h.Provider.ListLocalDeals()
			require.NoError(t, err)
			if len(pds) == 0 {
				t.Log("no deal created on provider after stopping")
			} else {
				t.Logf("provider state after stopping is %s", storagemarket.DealStates[pds[0].State])
			}

			wg.Add(1)
			_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
				if event == storagemarket.ProviderEventDealExpired {
					wg.Done()
				}
			})

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
			require.NoError(t, h.Provider.Start(ctx))
			require.NoError(t, h.Client.Start(ctx))
			t.Log("------- client and provider have been restarted---------")
			wg.Wait()
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

type harness struct {
	Ctx          context.Context
	Epoch        abi.ChainEpoch
	PayloadCid   cid.Cid
	StoreID      *multistore.StoreID
	ProviderAddr address.Address
	ClientAddr   address.Address
	Client       storagemarket.StorageClient
	ClientNode   *testnodes.FakeClientNode
	Provider     storagemarket.StorageProvider
	ProviderNode *testnodes.FakeProviderNode
	SMState      *testnodes.StorageMarketState
	ProviderInfo storagemarket.StorageProviderInfo
	TestData     *shared_testutil.Libp2pTestData
	TempFilePath string

	ClientDelay   testnodes.DelayFakeCommonNode
	ProviderDelay testnodes.DelayFakeCommonNode
}

func newHarness(t *testing.T, ctx context.Context, useStore bool, clientDelay testnodes.DelayFakeCommonNode,
	providerDelay testnodes.DelayFakeCommonNode) *harness {
	smState := testnodes.NewStorageMarketState()
	return newHarnessWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), smState, useStore, "", clientDelay, providerDelay)
}

func (h *harness) createNewProvider(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, tempPath string) {
	gs2 := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host2), td.Loader2, td.Storer2)
	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTNet2, dtTransport2, td.DTStoredCounter2)
	require.NoError(t, err)
	err = dt2.Start(ctx)
	require.NoError(t, err)

	storedAsk, err := storedask.NewStoredAsk(td.Ds2, datastore.NewKey("latest-ask"), h.ProviderNode, h.ProviderAddr)
	require.NoError(t, err)
	providerDealFunds, err := funds.NewDealFunds(td.Ds2, datastore.NewKey("storage/provider/dealfunds"))
	require.NoError(t, err)

	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	require.NoError(t, err)

	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, network.RetryParameters(0, 0, 0)),
		td.Ds2,
		fs,
		td.MultiStore2,
		piecestore.NewPieceStore(td.Ds2),
		dt2,
		h.ProviderNode,
		h.ProviderAddr,
		abi.RegisteredSealProof_StackedDrg2KiBV1,
		storedAsk,
		providerDealFunds,
	)
	require.NoError(t, err)
	h.Provider = provider
}

func newHarnessWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState, useStore bool, tempPath string,
	clientDelay testnodes.DelayFakeCommonNode, providerDelay testnodes.DelayFakeCommonNode) *harness {

	clientDelay.OnDealSectorCommittedChan = make(chan struct{})
	clientDelay.OnDealExpiredOrSlashedChan = make(chan struct{})
	clientDelay.ValidatePublishedDealChan = make(chan struct{})

	providerDelay.OnDealSectorCommittedChan = make(chan struct{})
	providerDelay.OnDealExpiredOrSlashedChan = make(chan struct{})
	providerDelay.ValidatePublishedDealChan = make(chan struct{})

	epoch := abi.ChainEpoch(100)
	fpath := filepath.Join("storagemarket", "fixtures", "payload.txt")
	var rootLink ipld.Link
	var storeID *multistore.StoreID
	if useStore {
		var id multistore.StoreID
		rootLink, id = td.LoadUnixFSFileToStore(t, fpath, false)
		storeID = &id
	} else {
		rootLink = td.LoadUnixFSFile(t, fpath, false)
	}
	payloadCid := rootLink.(cidlink.Link).Cid

	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: testnodes.FakeCommonNode{SMState: smState,
			DelayFakeCommonNode: clientDelay},
		ClientAddr:         address.TestAddress,
		ExpectedMinerInfos: []address.Address{address.TestAddress2},
	}

	expDealID := abi.DealID(rand.Uint64())
	psdReturn := market.PublishStorageDealsReturn{IDs: []abi.DealID{expDealID}}
	psdReturnBytes := bytes.NewBuffer([]byte{})
	err := psdReturn.MarshalCBOR(psdReturnBytes)
	assert.NoError(t, err)

	providerAddr := address.TestAddress2

	if len(tempPath) == 0 {
		tempPath, err = ioutil.TempDir("", "storagemarket_test")
		assert.NoError(t, err)
	}

	ps := piecestore.NewPieceStore(td.Ds2)
	providerNode := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			DelayFakeCommonNode:    providerDelay,
			SMState:                smState,
			WaitForMessageRetBytes: psdReturnBytes.Bytes(),
		},
		MinerAddr: providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	gs1 := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host1), td.Loader1, td.Storer1)
	dtTransport1 := dtgstransport.NewTransport(td.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(td.DTStore1, td.DTNet1, dtTransport1, td.DTStoredCounter1)
	require.NoError(t, err)
	err = dt1.Start(ctx)
	require.NoError(t, err)
	clientDealFunds, err := funds.NewDealFunds(td.Ds1, datastore.NewKey("storage/client/dealfunds"))
	require.NoError(t, err)

	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1, network.RetryParameters(0, 0, 0)),
		td.Bs1,
		td.MultiStore1,
		dt1,
		discovery.NewLocal(td.Ds1),
		td.Ds1,
		&clientNode,
		clientDealFunds,
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)

	gs2 := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host2), td.Loader2, td.Storer2)
	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTNet2, dtTransport2, td.DTStoredCounter2)
	require.NoError(t, err)
	err = dt2.Start(ctx)
	require.NoError(t, err)

	storedAsk, err := storedask.NewStoredAsk(td.Ds2, datastore.NewKey("latest-ask"), providerNode, providerAddr)
	assert.NoError(t, err)
	providerDealFunds, err := funds.NewDealFunds(td.Ds2, datastore.NewKey("storage/provider/dealfunds"))
	assert.NoError(t, err)

	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, network.RetryParameters(0, 0, 0)),
		td.Ds2,
		fs,
		td.MultiStore2,
		ps,
		dt2,
		providerNode,
		providerAddr,
		abi.RegisteredSealProof_StackedDrg2KiBV1,
		storedAsk,
		providerDealFunds,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
	assert.NoError(t, err)

	// Closely follows the MinerInfo struct in the spec
	providerInfo := storagemarket.StorageProviderInfo{
		Address:    providerAddr,
		Owner:      providerAddr,
		Worker:     providerAddr,
		SectorSize: 1 << 20,
		PeerID:     td.Host2.ID(),
	}

	smState.Providers = map[address.Address]*storagemarket.StorageProviderInfo{providerAddr: &providerInfo}
	return &harness{
		Ctx:           ctx,
		Epoch:         epoch,
		PayloadCid:    payloadCid,
		StoreID:       storeID,
		ClientAddr:    clientNode.ClientAddr,
		ProviderAddr:  providerAddr,
		Client:        client,
		ClientNode:    &clientNode,
		Provider:      provider,
		ProviderNode:  providerNode,
		ProviderInfo:  providerInfo,
		TestData:      td,
		SMState:       smState,
		TempFilePath:  tempPath,
		ClientDelay:   clientDelay,
		ProviderDelay: providerDelay,
	}
}

func (h *harness) ProposeStorageDeal(t *testing.T, dataRef *storagemarket.DataRef, fastRetrieval, verifiedDeal bool) *storagemarket.ProposeStorageDealResult {
	result, err := h.Client.ProposeStorageDeal(h.Ctx, storagemarket.ProposeStorageDealParams{
		Addr:          h.ClientAddr,
		Info:          &h.ProviderInfo,
		Data:          dataRef,
		StartEpoch:    h.Epoch + 100,
		EndEpoch:      h.Epoch + 20100,
		Price:         big.NewInt(1),
		Collateral:    big.NewInt(0),
		Rt:            abi.RegisteredSealProof_StackedDrg2KiBV1,
		FastRetrieval: fastRetrieval,
		VerifiedDeal:  verifiedDeal,
		StoreID:       h.StoreID,
	})
	assert.NoError(t, err)
	return result
}

func (h *harness) WaitForProviderEvent(wg *sync.WaitGroup, waitEvent storagemarket.ProviderEvent) {
	wg.Add(1)
	h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}

func (h *harness) WaitForClientEvent(wg *sync.WaitGroup, waitEvent storagemarket.ClientEvent) {
	wg.Add(1)
	h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}
