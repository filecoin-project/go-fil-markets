package testharness

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnetwork "github.com/ipfs/go-graphsync/network"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/protocol"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	"github.com/filecoin-project/go-data-transfer/testutil"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

type StorageHarness struct {
	*dependencies.StorageDependencies
	PayloadCid   cid.Cid
	StoreID      *multistore.StoreID
	Client       storagemarket.StorageClient
	Provider     storagemarket.StorageProvider
	TempFilePath string
}

func NewHarness(t *testing.T, ctx context.Context, useStore bool, cd testnodes.DelayFakeCommonNode, pd testnodes.DelayFakeCommonNode,
	disableNewDeals bool) *StorageHarness {
	smState := testnodes.NewStorageMarketState()
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	deps := dependencies.NewDependenciesWithTestData(t, ctx, td, smState, "", cd, pd)
	return NewHarnessWithTestData(t, td, deps, useStore, disableNewDeals)
}

func NewHarnessWithTestData(t *testing.T, td *shared_testutil.Libp2pTestData, deps *dependencies.StorageDependencies, useStore bool, disableNewDeals bool) *StorageHarness {
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

	// create provider and client

	clientDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/client"))
	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1, network.RetryParameters(0, 0, 0, 0)),
		td.Bs1,
		td.MultiStore1,
		deps.DTClient,
		deps.PeerResolver,
		clientDs,
		deps.ClientNode,
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)

	providerDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/provider"))
	networkOptions := []network.Option{network.RetryParameters(0, 0, 0, 0)}
	if disableNewDeals {
		networkOptions = append(networkOptions,
			network.SupportedAskProtocols([]protocol.ID{storagemarket.OldAskProtocolID}),
			network.SupportedDealProtocols([]protocol.ID{storagemarket.OldDealProtocolID}),
			network.SupportedDealStatusProtocols([]protocol.ID{storagemarket.OldDealStatusProtocolID}),
		)
	}
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, networkOptions...),
		providerDs,
		deps.Fs,
		td.MultiStore2,
		deps.PieceStore,
		deps.DTProvider,
		deps.ProviderNode,
		deps.ProviderAddr,
		deps.StoredAsk,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	assert.NoError(t, err)

	return &StorageHarness{
		StorageDependencies: deps,
		PayloadCid:          payloadCid,
		StoreID:             storeID,
		Client:              client,
		Provider:            provider,
	}
}

func (h *StorageHarness) CreateNewProvider(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData) storagemarket.StorageProvider {
	gs2 := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host2), td.Loader2, td.Storer2)
	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTTmpDir2, td.DTNet2, dtTransport2)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt2)

	providerDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/provider"))
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, network.RetryParameters(0, 0, 0, 0)),
		providerDs,
		h.Fs,
		td.MultiStore2,
		h.PieceStore,
		dt2,
		h.ProviderNode,
		h.ProviderAddr,
		h.StoredAsk,
	)
	require.NoError(t, err)
	return provider
}

func (h *StorageHarness) ProposeStorageDeal(t *testing.T, dataRef *storagemarket.DataRef, fastRetrieval, verifiedDeal bool) *storagemarket.ProposeStorageDealResult {
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

func (h *StorageHarness) WaitForProviderEvent(wg *sync.WaitGroup, waitEvent storagemarket.ProviderEvent) {
	wg.Add(1)
	h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}

func (h *StorageHarness) WaitForClientEvent(wg *sync.WaitGroup, waitEvent storagemarket.ClientEvent) {
	wg.Add(1)
	h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}

type StorageInstanceGenerator struct {
	ctx     context.Context
	t       *testing.T
	rnd     *rand.Rand
	mn      mocknet.Mocknet
	smState *testnodes.StorageMarketState
}

func NewStorageInstanceGenerator(ctx context.Context, t *testing.T, rnd *rand.Rand, mn mocknet.Mocknet, smState *testnodes.StorageMarketState) *StorageInstanceGenerator {
	return &StorageInstanceGenerator{
		ctx:     ctx,
		t:       t,
		rnd:     rnd,
		mn:      mn,
		smState: smState,
	}
}

type ClientHarness struct {
	Client       *storageimpl.Client
	Addr         address.Address
	NodeDeps     *shared_testutil.Libp2pNodeDeps
	DataTransfer datatransfer.Manager
	PeerResolver *discoveryimpl.Local
}

func (g *StorageInstanceGenerator) NewClient(td *shared_testutil.Libp2pNodeDeps, dt datatransfer.Manager) *ClientHarness {
	ctx := g.ctx
	t := g.t
	//td := shared_testutil.NewLibp2pNodeDeps(t, g.rnd, g.mn)

	//dt := CreateAndStartDataTransfer(ctx, t, td, restartConf)
	discovery, err := discoveryimpl.NewLocal(namespace.Wrap(td.Dstore, datastore.NewKey("/deals/local")))
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, discovery)

	clientAddr := randAddress(t, g.rnd)
	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState:             g.smState,
			DealFunds:           shared_testutil.NewTestDealFunds(),
			DelayFakeCommonNode: testnodes.DelayFakeCommonNode{},
		},
		ClientAddr:         clientAddr,
		ExpectedMinerInfos: []address.Address{address.TestAddress2},
	}

	clientDs := namespace.Wrap(td.Dstore, datastore.NewKey("/deals/client"))
	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host, network.RetryParameters(0, 0, 0, 0)),
		td.Bstore,
		td.MultiStore,
		dt,
		discovery,
		clientDs,
		&clientNode,
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)
	return &ClientHarness{
		Client:   client,
		Addr:     clientAddr,
		NodeDeps: td,
		//DataTransfer: dt,
		PeerResolver: discovery,
	}
}

type ProviderHarness struct {
	Provider     storagemarket.StorageProvider
	ProviderInfo storagemarket.StorageProviderInfo
	NodeDeps     *shared_testutil.Libp2pNodeDeps
	DataTransfer datatransfer.Manager
	PieceStore   piecestore.PieceStore
	ProviderNode *testnodes.FakeProviderNode
}

func (g *StorageInstanceGenerator) NewProvider() *ProviderHarness {
	ctx := g.ctx
	t := g.t
	td := shared_testutil.NewLibp2pNodeDeps(t, g.rnd, g.mn)

	dt := CreateAndStartDataTransfer(ctx, t, td)

	tempPath, err := ioutil.TempDir("", "storagemarket_test")
	assert.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tempPath) })

	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	ps, err := piecestoreimpl.NewPieceStore(td.Dstore)
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, ps)

	expDealID := abi.DealID(rand.Uint64())
	psdReturn := market.PublishStorageDealsReturn{IDs: []abi.DealID{expDealID}}
	psdReturnBytes := bytes.NewBuffer([]byte{})
	err = psdReturn.MarshalCBOR(psdReturnBytes)
	assert.NoError(t, err)

	providerAddr := randAddress(t, g.rnd)
	providerNode := testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			DelayFakeCommonNode:    testnodes.DelayFakeCommonNode{},
			SMState:                g.smState,
			DealFunds:              shared_testutil.NewTestDealFunds(),
			WaitForMessageRetBytes: psdReturnBytes.Bytes(),
		},
		MinerAddr: providerAddr,
	}

	storedAskDs := namespace.Wrap(td.Dstore, datastore.NewKey("/storage/ask"))
	storedAsk, err := storedask.NewStoredAsk(storedAskDs, datastore.NewKey("latest-ask"), &providerNode, providerAddr)
	assert.NoError(t, err)

	// Closely follows the MinerInfo struct in the spec
	providerInfo := storagemarket.StorageProviderInfo{
		Address:    providerAddr,
		Owner:      providerAddr,
		Worker:     providerAddr,
		SectorSize: 1 << 20,
		PeerID:     td.Host.ID(),
	}
	g.smState.AddProvider(providerAddr, &providerInfo)

	providerDs := namespace.Wrap(td.Dstore, datastore.NewKey("/deals/provider"))
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host, network.RetryParameters(0, 0, 0, 0)),
		providerDs,
		fs,
		td.MultiStore,
		ps,
		dt,
		&providerNode,
		providerAddr,
		storedAsk,
	)
	require.NoError(t, err)
	return &ProviderHarness{
		Provider:     provider,
		NodeDeps:     td,
		ProviderInfo: providerInfo,
		DataTransfer: dt,
		PieceStore:   ps,
		ProviderNode: &providerNode,
	}
}

func randAddress(t *testing.T, rnd *rand.Rand) address.Address {
	provAddrBytes := make([]byte, 16)
	rnd.Read(provAddrBytes)
	providerAddr, err := address.NewActorAddress(provAddrBytes)
	require.NoError(t, err)
	return providerAddr
}

func CreateAndStartDataTransfer(ctx context.Context, t *testing.T, td *shared_testutil.Libp2pNodeDeps, opts ...dtimpl.DataTransferOption) datatransfer.Manager {
	gs := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host), td.Loader, td.Storer)
	dtTransport := dtgstransport.NewTransport(td.Host.ID(), gs)
	dt, err := dtimpl.NewDataTransfer(td.DTStore, td.DTTmpDir, td.DTNet, dtTransport, opts...)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt)
	return dt
}
