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

	"github.com/filecoin-project/go-address"
	graphsync "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestMakeDeal(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, ctx)
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
			require.Equal(t, deal, unmDeal)
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
	err := h.Provider.SetAsk(big.NewInt(0), 50_000)
	assert.NoError(t, err)

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid})
	proposalCid := result.ProposalCid

	time.Sleep(time.Millisecond * 200)

	ctx, canc := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer canc()
	var providerSeenDeal storagemarket.MinerDeal
	var clientSeenDeal storagemarket.ClientDeal
	var providerstates, clientstates []storagemarket.StorageDealStatus
	for providerSeenDeal.State != storagemarket.StorageDealCompleted ||
		clientSeenDeal.State != storagemarket.StorageDealActive {
		select {
		case clientSeenDeal = <-clientDealChan:
			clientstates = append(clientstates, clientSeenDeal.State)
		case providerSeenDeal = <-providerDealChan:
			providerstates = append(providerstates, providerSeenDeal.State)
		case <-ctx.Done():
			t.Fatalf("deal incomplete, client deal state: %s (%d), provider deal state: %s (%d)",
				storagemarket.DealStates[clientSeenDeal.State],
				clientSeenDeal.State,
				storagemarket.DealStates[providerSeenDeal.State],
				providerSeenDeal.State,
			)
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
		storagemarket.StorageDealActive,
		storagemarket.StorageDealCompleted,
	}

	expClientStates := []storagemarket.StorageDealStatus{
		storagemarket.StorageDealEnsureClientFunds,
		//storagemarket.StorageDealClientFunding,  // skipped because funds available
		storagemarket.StorageDealFundsEnsured,
		storagemarket.StorageDealWaitingForDataRequest,
		storagemarket.StorageDealTransferring,
		storagemarket.StorageDealValidating,
		storagemarket.StorageDealProposalAccepted,
		storagemarket.StorageDealSealing,
		storagemarket.StorageDealActive,
	}

	assert.Equal(t, expProviderStates, providerstates)
	assert.Equal(t, expClientStates, clientstates)

	// check a couple of things to make sure we're getting the whole deal
	assert.Equal(t, h.TestData.Host1.ID(), providerSeenDeal.Client)
	assert.Empty(t, providerSeenDeal.Message)
	assert.Equal(t, proposalCid, providerSeenDeal.ProposalCid)
	assert.Equal(t, h.ProviderAddr, providerSeenDeal.ClientDealProposal.Proposal.Provider)

	cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealActive, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCompleted, pd.State)
}

func TestMakeDealOffline(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, ctx)
	require.NoError(t, h.Client.Start(ctx))

	carBuf := new(bytes.Buffer)

	err := cario.NewCarIO().WriteCar(ctx, h.TestData.Bs1, h.PayloadCid, shared.AllSelector(), carBuf)
	require.NoError(t, err)

	commP, size, err := pieceio.GeneratePieceCommitment(abi.RegisteredProof_StackedDRG2KiBPoSt, carBuf, uint64(carBuf.Len()))
	assert.NoError(t, err)

	dataRef := &storagemarket.DataRef{
		TransferType: storagemarket.TTManual,
		Root:         h.PayloadCid,
		PieceCid:     &commP,
		PieceSize:    size,
	}

	result := h.ProposeStorageDeal(t, dataRef)
	proposalCid := result.ProposalCid

	time.Sleep(time.Millisecond * 100)

	cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealValidating, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	shared_testutil.AssertDealState(t, storagemarket.StorageDealWaitingForData, pd.State)

	err = cario.NewCarIO().WriteCar(ctx, h.TestData.Bs1, h.PayloadCid, shared.AllSelector(), carBuf)
	require.NoError(t, err)
	err = h.Provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealActive, cd.State)

	providerDeals, err = h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd = providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCompleted, pd.State)
}

func TestMakeDealNonBlocking(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, ctx)
	testCids := shared_testutil.GenerateCids(2)

	h.ProviderNode.WaitForMessageBlocks = true
	h.ProviderNode.AddFundsCid = testCids[1]
	require.NoError(t, h.Provider.Start(ctx))

	h.ClientNode.AddFundsCid = testCids[0]
	require.NoError(t, h.Client.Start(ctx))

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid})

	time.Sleep(time.Millisecond * 500)

	cd, err := h.Client.GetLocalDeal(ctx, result.ProposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealValidating, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	// Provider should be blocking on waiting for funds to appear on chain
	pd := providerDeals[0]
	assert.Equal(t, result.ProposalCid, pd.ProposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealProviderFunding, pd.State)
}

func TestRestartClient(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, ctx)

	require.NoError(t, h.Provider.Start(ctx))
	require.NoError(t, h.Client.Start(ctx))

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), 50_000)
	assert.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)
	_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventFundsEnsured {
			// Stop the client and provider at some point during deal negotiation
			require.NoError(t, h.Client.Stop())
			require.NoError(t, h.Provider.Stop())
			wg.Done()
		}
	})

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid})
	proposalCid := result.ProposalCid

	wg.Wait()

	cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	assert.NotEqual(t, storagemarket.StorageDealActive, cd.State)

	h = newHarnessWithTestData(t, ctx, h.TestData, h.SMState)

	wg.Add(1)
	_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventDealActivated {
			wg.Done()
		}
	})

	wg.Add(1)
	_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == storagemarket.ProviderEventDealActivated {
			wg.Done()
		}
	})

	require.NoError(t, h.Provider.Start(ctx))
	require.NoError(t, h.Client.Start(ctx))

	wg.Wait()

	cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealActive, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealActive, pd.State)
}

type harness struct {
	Ctx          context.Context
	Epoch        abi.ChainEpoch
	PayloadCid   cid.Cid
	ProviderAddr address.Address
	Client       storagemarket.StorageClient
	ClientNode   *testnodes.FakeClientNode
	Provider     storagemarket.StorageProvider
	ProviderNode *testnodes.FakeProviderNode
	SMState      *testnodes.StorageMarketState
	ProviderInfo storagemarket.StorageProviderInfo
	TestData     *shared_testutil.Libp2pTestData
}

func newHarness(t *testing.T, ctx context.Context) *harness {
	smState := testnodes.NewStorageMarketState()
	return newHarnessWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), smState)
}

func newHarnessWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState) *harness {
	epoch := abi.ChainEpoch(100)
	fpath := filepath.Join("storagemarket", "fixtures", "payload.txt")
	rootLink := td.LoadUnixFSFile(t, fpath, false)
	payloadCid := rootLink.(cidlink.Link).Cid

	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: testnodes.FakeCommonNode{SMState: smState},
		ClientAddr:     address.TestAddress,
	}

	expDealID := abi.DealID(rand.Uint64())
	psdReturn := market.PublishStorageDealsReturn{IDs: []abi.DealID{expDealID}}
	psdReturnBytes := bytes.NewBuffer([]byte{})
	err := psdReturn.MarshalCBOR(psdReturnBytes)
	assert.NoError(t, err)

	providerAddr := address.TestAddress2
	tempPath, err := ioutil.TempDir("", "storagemarket_test")
	assert.NoError(t, err)
	ps := piecestore.NewPieceStore(td.Ds2)
	providerNode := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState:                smState,
			WaitForMessageRetBytes: psdReturnBytes.Bytes(),
		},
		MinerAddr: providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	dt1 := graphsync.NewGraphSyncDataTransfer(td.Host1, td.GraphSync1, td.DTStoredCounter1)
	require.NoError(t, dt1.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, &shared_testutil.FakeDTValidator{}))

	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1),
		td.Bs1,
		dt1,
		discovery.NewLocal(td.Ds1),
		td.Ds1,
		&clientNode,
	)
	require.NoError(t, err)

	dt2 := graphsync.NewGraphSyncDataTransfer(td.Host2, td.GraphSync2, td.DTStoredCounter2)
	require.NoError(t, dt2.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, &shared_testutil.FakeDTValidator{}))

	storedAsk, err := storedask.NewStoredAsk(td.Ds2, datastore.NewKey("latest-ask"), providerNode, providerAddr)
	assert.NoError(t, err)
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2),
		td.Ds2,
		td.Bs2,
		fs,
		ps,
		dt2,
		providerNode,
		providerAddr,
		abi.RegisteredProof_StackedDRG2KiBPoSt,
		storedAsk,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.SetAsk(big.NewInt(0), 50_000)
	assert.NoError(t, err)

	err = provider.Start(ctx)
	assert.NoError(t, err)

	// Closely follows the MinerInfo struct in the spec
	providerInfo := storagemarket.StorageProviderInfo{
		Address:    providerAddr,
		Owner:      providerAddr,
		Worker:     providerAddr,
		SectorSize: 1 << 20,
		PeerID:     td.Host2.ID(),
	}

	return &harness{
		Ctx:          ctx,
		Epoch:        epoch,
		PayloadCid:   payloadCid,
		ProviderAddr: providerAddr,
		Client:       client,
		ClientNode:   &clientNode,
		Provider:     provider,
		ProviderNode: providerNode,
		ProviderInfo: providerInfo,
		TestData:     td,
		SMState:      smState,
	}
}

func (h *harness) ProposeStorageDeal(t *testing.T, dataRef *storagemarket.DataRef) *storagemarket.ProposeStorageDealResult {
	result, err := h.Client.ProposeStorageDeal(
		h.Ctx,
		h.ProviderAddr,
		&h.ProviderInfo,
		dataRef,
		h.Epoch+100,
		h.Epoch+20100,
		big.NewInt(1),
		big.NewInt(0),
		abi.RegisteredProof_StackedDRG2KiBPoSt,
	)
	assert.NoError(t, err)
	return result
}
