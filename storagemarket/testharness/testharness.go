package testharness

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type StorageHarness struct {
	*dependencies.StorageDependencies
	PayloadCid   cid.Cid
	StoreID      *multistore.StoreID
	Client       storagemarket.StorageClient
	Provider     storagemarket.StorageProvider
	TempFilePath string
}

func NewHarness(t *testing.T, ctx context.Context, useStore bool, d testnodes.DelayFakeCommonNode, disableNewDeals bool) *StorageHarness {
	smState := testnodes.NewStorageMarketState()
	return NewHarnessWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), smState, useStore, "", d, disableNewDeals)
}

func NewHarnessWithTestData(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData, smState *testnodes.StorageMarketState, useStore bool, tempPath string,
	delayFakeEnvNode testnodes.DelayFakeCommonNode, disableNewDeals bool) *StorageHarness {
	deps := dependencies.NewDependenciesWithTestData(t, ctx, td, smState, tempPath, delayFakeEnvNode)
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
		network.NewFromLibp2pHost(td.Host1, network.RetryParameters(0, 0, 0)),
		td.Bs1,
		td.MultiStore1,
		deps.DTClient,
		deps.PeerResolver,
		clientDs,
		deps.ClientNode,
		deps.ClientDealFunds,
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)

	providerDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/provider"))
	networkOptions := []network.Option{network.RetryParameters(0, 0, 0)}
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
		abi.RegisteredSealProof_StackedDrg2KiBV1,
		deps.StoredAsk,
		deps.ProviderDealFunds,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.SetAsk(big.NewInt(0), big.NewInt(0), 50_000)
	assert.NoError(t, err)

	return &StorageHarness{
		StorageDependencies: deps,
		PayloadCid:          payloadCid,
		StoreID:             storeID,
		Client:              client,
		Provider:            provider,
	}
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
