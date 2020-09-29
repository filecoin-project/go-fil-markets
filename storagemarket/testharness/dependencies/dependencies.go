package dependencies

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StorageDependencies are the dependencies required to initialize a storage client/provider
type StorageDependencies struct {
	Ctx                 context.Context
	Epoch               abi.ChainEpoch
	ProviderAddr        address.Address
	ClientAddr          address.Address
	ClientNode          *testnodes.FakeClientNode
	ProviderNode        *testnodes.FakeProviderNode
	SMState             *testnodes.StorageMarketState
	TempFilePath        string
	ProviderInfo        storagemarket.StorageProviderInfo
	TestData            *shared_testutil.Libp2pTestData
	PieceStore          piecestore.PieceStore
	DTClient            datatransfer.Manager
	DTProvider          datatransfer.Manager
	PeerResolver        *discoveryimpl.Local
	DelayFakeCommonNode testnodes.DelayFakeCommonNode
	Fs                  filestore.FileStore
	ClientDealFunds     funds.DealFunds
	StoredAsk           *storedask.StoredAsk
	ProviderDealFunds   funds.DealFunds
}

func NewDependenciesWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState, tempPath string,
	delayFakeEnvNode testnodes.DelayFakeCommonNode) *StorageDependencies {

	delayFakeEnvNode.OnDealSectorCommittedChan = make(chan struct{})
	delayFakeEnvNode.OnDealExpiredOrSlashedChan = make(chan struct{})

	epoch := abi.ChainEpoch(100)

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

	ps, err := piecestoreimpl.NewPieceStore(td.Ds2)
	shared_testutil.StartAndWaitForReady(ctx, t, ps)

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

	discovery, err := discoveryimpl.NewLocal(namespace.Wrap(td.Ds1, datastore.NewKey("/deals/local")))
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, discovery)

	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), td.GraphSync2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTNet2, dtTransport2, td.DTStoredCounter2)
	require.NoError(t, err)
	err = dt2.Start(ctx)
	require.NoError(t, err)

	storedAsk, err := storedask.NewStoredAsk(td.Ds2, datastore.NewKey("latest-ask"), providerNode, providerAddr)
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
		Ctx:                 ctx,
		Epoch:               epoch,
		ClientAddr:          clientNode.ClientAddr,
		ProviderAddr:        providerAddr,
		ClientNode:          &clientNode,
		ProviderNode:        providerNode,
		ProviderInfo:        providerInfo,
		TestData:            td,
		SMState:             smState,
		TempFilePath:        tempPath,
		DelayFakeCommonNode: delayFakeEnvNode,
		DTClient:            dt1,
		DTProvider:          dt2,
		PeerResolver:        discovery,
		PieceStore:          ps,
		Fs:                  fs,
		ClientDealFunds:     clientDealFunds,
		StoredAsk:           storedAsk,
		ProviderDealFunds:   providerDealFunds,
	}
}
