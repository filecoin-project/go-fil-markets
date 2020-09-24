package storagemarket_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestRestartClient(t *testing.T) {
	testCases := map[string]struct {
		clientStopEvent   storagemarket.ClientEvent
		providerStopEvent storagemarket.ProviderEvent
		fh                func(h *harness)
	}{
		"ClientEventFundsEnsured": {
			clientStopEvent: storagemarket.ClientEventFundsEnsured,
		},
		"ClientEventInitiateDataTransfer": {
			clientStopEvent: storagemarket.ClientEventInitiateDataTransfer,
		},
		"ClientEventDataTransferInitiated": {
			clientStopEvent: storagemarket.ClientEventDataTransferInitiated,
		},
		"ClientEventDataTransferComplete": {
			clientStopEvent:   storagemarket.ClientEventDataTransferComplete,
			providerStopEvent: storagemarket.ProviderEventDataTransferCompleted,
		},
		"ClientEventDealAccepted": {
			clientStopEvent: storagemarket.ClientEventDealAccepted,
		},
		"ClientEventDealActivated": {
			clientStopEvent: storagemarket.ClientEventDealActivated,
			fh: func(h *harness) {
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
				h.DelayFakeCommonNode.OnDealSectorCommittedChan <- struct{}{}
			},
		},
		"ClientEventDealPublished": {
			clientStopEvent: storagemarket.ClientEventDealPublished,
			fh: func(h *harness) {
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
			h := newHarness(t, ctx, true, testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true,
				OnDealSectorCommitted: true})

			require.NoError(t, h.Provider.Start(ctx))
			require.NoError(t, h.Client.Start(ctx))

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
			assert.NoError(t, err)

			wg := sync.WaitGroup{}
			wg.Add(1)
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == tc.clientStopEvent {
					require.NoError(t, h.Client.Stop())

					if tc.providerStopEvent == 0 {
						require.NoError(t, h.Provider.Stop())
					}

					wg.Done()
				}
			})

			if tc.providerStopEvent != 0 {
				wg.Add(1)
				_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if event == tc.providerStopEvent {
						require.NoError(t, h.Provider.Stop())
						wg.Done()
					}
				})
			}

			result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
			proposalCid := result.ProposalCid

			if tc.fh != nil {
				tc.fh(h)
			}

			wg.Wait()

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			if tc.clientStopEvent != storagemarket.ClientEventDealActivated && tc.clientStopEvent != storagemarket.ClientEventDealPublished {
				assert.NotEqual(t, storagemarket.StorageDealActive, cd.State)
			}
			h = newHarnessWithTestData(t, ctx, h.TestData, h.SMState, true, h.TempFilePath, testnodes.DelayFakeCommonNode{})

			// deal could have expired already on the provider side for the `ClientEventDealAccepted` event
			// so, we should wait on the `ProviderEventDealExpired` event ONLY if the deal has not expired.
			providerState, err := h.Provider.ListLocalDeals()
			assert.NoError(t, err)

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

			require.NoError(t, h.Provider.Start(ctx))
			require.NoError(t, h.Client.Start(ctx))

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

	DelayFakeCommonNode testnodes.DelayFakeCommonNode
}

func newHarness(t *testing.T, ctx context.Context, useStore bool, d testnodes.DelayFakeCommonNode) *harness {
	smState := testnodes.NewStorageMarketState()
	return newHarnessWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), smState, useStore, "", d)
}

func newHarnessWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState, useStore bool, tempPath string,
	delayFakeEnvNode testnodes.DelayFakeCommonNode) *harness {

	delayFakeEnvNode.OnDealSectorCommittedChan = make(chan struct{})
	delayFakeEnvNode.OnDealExpiredOrSlashedChan = make(chan struct{})

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
			DelayFakeCommonNode: delayFakeEnvNode},
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
			DelayFakeCommonNode:    delayFakeEnvNode,
			SMState:                smState,
			WaitForMessageRetBytes: psdReturnBytes.Bytes(),
		},
		MinerAddr: providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	dtTransport1 := dtgstransport.NewTransport(td.Host1.ID(), td.GraphSync1)
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
		storageimpl.DealPollingInterval(5*time.Millisecond),
	)
	require.NoError(t, err)

	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), td.GraphSync2)
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
		Ctx:                 ctx,
		Epoch:               epoch,
		PayloadCid:          payloadCid,
		StoreID:             storeID,
		ClientAddr:          clientNode.ClientAddr,
		ProviderAddr:        providerAddr,
		Client:              client,
		ClientNode:          &clientNode,
		Provider:            provider,
		ProviderNode:        providerNode,
		ProviderInfo:        providerInfo,
		TestData:            td,
		SMState:             smState,
		TempFilePath:        tempPath,
		DelayFakeCommonNode: delayFakeEnvNode,
	}
}

func (h *harness) ProposeStorageDeal(t *testing.T, dataRef *storagemarket.DataRef, fastRetrieval, verifiedDeal bool) *storagemarket.ProposeStorageDealResult {
	var dealDuration = abi.ChainEpoch(180 * builtin.EpochsInDay)

	result, err := h.Client.ProposeStorageDeal(h.Ctx, storagemarket.ProposeStorageDealParams{
		Addr:          h.ClientAddr,
		Info:          &h.ProviderInfo,
		Data:          dataRef,
		StartEpoch:    h.Epoch + 100,
		EndEpoch:      h.Epoch + 100 + dealDuration,
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
