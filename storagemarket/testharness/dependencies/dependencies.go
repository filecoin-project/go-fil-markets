package dependencies

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	"github.com/filecoin-project/go-data-transfer/testutil"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

// StorageDependencies are the dependencies required to initialize a storage client/provider
type StorageDependencies struct {
	Ctx                               context.Context
	Epoch                             abi.ChainEpoch
	ProviderAddr                      address.Address
	ClientAddr                        address.Address
	ClientNode                        *testnodes.FakeClientNode
	ProviderNode                      *testnodes.FakeProviderNode
	SMState                           *testnodes.StorageMarketState
	TempFilePath                      string
	ProviderInfo                      storagemarket.StorageProviderInfo
	TestData                          *shared_testutil.Libp2pTestData
	PieceStore                        piecestore.PieceStore
	DTClient                          datatransfer.Manager
	DTProvider                        datatransfer.Manager
	PeerResolver                      *discoveryimpl.Local
	ClientDelayFakeCommonNode         testnodes.DelayFakeCommonNode
	ProviderClientDelayFakeCommonNode testnodes.DelayFakeCommonNode
	Fs                                filestore.FileStore
	ClientDealFunds                   funds.DealFunds
	StoredAsk                         *storedask.StoredAsk
	ProviderDealFunds                 funds.DealFunds
}

func NewDependenciesWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState, tempPath string,
	cd testnodes.DelayFakeCommonNode, pd testnodes.DelayFakeCommonNode) *StorageDependencies {

	cd.OnDealSectorCommittedChan = make(chan struct{})
	cd.OnDealExpiredOrSlashedChan = make(chan struct{})

	pd.OnDealSectorCommittedChan = make(chan struct{})
	pd.OnDealExpiredOrSlashedChan = make(chan struct{})

	epoch := abi.ChainEpoch(100)

	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: testnodes.FakeCommonNode{SMState: smState,
			DelayFakeCommonNode: cd},
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

	ps, err := piecestoreimpl.NewPieceStore(td.Ds2)
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, ps)

	providerNode := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			DelayFakeCommonNode:    pd,
			SMState:                smState,
			WaitForMessageRetBytes: psdReturnBytes.Bytes(),
		},
		MinerAddr: providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client

	gs1 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(td.Host1), td.Loader1, td.Storer1)
	dtTransport1 := dtgstransport.NewTransport(td.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(td.DTStore1, td.DTNet1, dtTransport1, td.DTStoredCounter1)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt1)

	require.NoError(t, err)
	clientDealFunds, err := funds.NewDealFunds(td.Ds1, datastore.NewKey("storage/client/dealfunds"))
	require.NoError(t, err)

	discovery, err := discoveryimpl.NewLocal(namespace.Wrap(td.Ds1, datastore.NewKey("/deals/local")))
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, discovery)

	gs2 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(td.Host2), td.Loader2, td.Storer2)
	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTNet2, dtTransport2, td.DTStoredCounter2)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt2)

	storedAskDs := namespace.Wrap(td.Ds2, datastore.NewKey("/storage/ask"))
	storedAsk, err := storedask.NewStoredAsk(storedAskDs, datastore.NewKey("latest-ask"), providerNode, providerAddr)
	assert.NoError(t, err)
	providerDealFunds, err := funds.NewDealFunds(td.Ds2, datastore.NewKey("storage/provider/dealfunds"))
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
	return &StorageDependencies{
		Ctx:                               ctx,
		Epoch:                             epoch,
		ClientAddr:                        clientNode.ClientAddr,
		ProviderAddr:                      providerAddr,
		ClientNode:                        &clientNode,
		ProviderNode:                      providerNode,
		ProviderInfo:                      providerInfo,
		TestData:                          td,
		SMState:                           smState,
		TempFilePath:                      tempPath,
		ClientDelayFakeCommonNode:         cd,
		ProviderClientDelayFakeCommonNode: pd,
		DTClient:                          dt1,
		DTProvider:                        dt2,
		PeerResolver:                      discovery,
		PieceStore:                        ps,
		Fs:                                fs,
		ClientDealFunds:                   clientDealFunds,
		StoredAsk:                         storedAsk,
		ProviderDealFunds:                 providerDealFunds,
	}
}
