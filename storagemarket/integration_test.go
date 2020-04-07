package storagemarket_test

import (
	"bytes"
	"context"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	graphsync "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestMakeDeal(t *testing.T) {
	ctx := context.Background()
	epoch := abi.ChainEpoch(100)
	nodeCommon := testnodes.FakeCommonNode{SMState: testnodes.NewStorageMarketState()}
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	rootLink := td.LoadUnixFSFile(t, "payload.txt", false)
	payloadCid := rootLink.(cidlink.Link).Cid

	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: nodeCommon,
		ClientAddr:     address.TestAddress,
	}

	providerAddr := address.TestAddress2
	tempPath, err := ioutil.TempDir("", "storagemarket_test")
	assert.NoError(t, err)
	ps := piecestore.NewPieceStore(td.Ds2)
	providerNode := testnodes.FakeProviderNode{
		FakeCommonNode: nodeCommon,
		MinerAddr:      providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	dt1 := graphsync.NewGraphSyncDataTransfer(td.Host1, td.GraphSync1)
	require.NoError(t, dt1.RegisterVoucherType(reflect.TypeOf(&requestvalidation.StorageDataTransferVoucher{}), &fakeDTValidator{}))

	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1),
		td.Bs1,
		dt1,
		discovery.NewLocal(td.Ds1),
		td.Ds1,
		&clientNode,
	)
	require.NoError(t, err)
	dt2 := graphsync.NewGraphSyncDataTransfer(td.Host2, td.GraphSync2)
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2),
		td.Ds2,
		td.Bs2,
		fs,
		ps,
		dt2,
		&providerNode,
		providerAddr,
		abi.RegisteredProof_StackedDRG2KiBPoSt,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.AddAsk(big.NewInt(0), 50_000)
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

	var proposalCid cid.Cid

	// make a deal
	client.Run(ctx)
	dataRef := &storagemarket.DataRef{
		TransferType: storagemarket.TTGraphsync,
		Root:         payloadCid,
	}
	result, err := client.ProposeStorageDeal(ctx, providerAddr, &providerInfo, dataRef, abi.ChainEpoch(epoch+100), abi.ChainEpoch(epoch+20100), big.NewInt(1), big.NewInt(0), abi.RegisteredProof_StackedDRG2KiBPoSt)
	assert.NoError(t, err)

	proposalCid = result.ProposalCid

	time.Sleep(time.Millisecond * 100)

	cd, err := client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	assert.Equal(t, cd.State, storagemarket.StorageDealActive)

	providerDeals, err := provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	assert.Equal(t, pd.State, storagemarket.StorageDealCompleted)
}

func TestMakeDealOffline(t *testing.T) {
	ctx := context.Background()
	epoch := abi.ChainEpoch(100)
	nodeCommon := testnodes.FakeCommonNode{SMState: testnodes.NewStorageMarketState()}
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	rootLink := td.LoadUnixFSFile(t, "payload.txt", false)
	payloadCid := rootLink.(cidlink.Link).Cid

	clientNode := testnodes.FakeClientNode{
		FakeCommonNode: nodeCommon,
		ClientAddr:     address.TestAddress,
	}

	providerAddr := address.TestAddress2
	tempPath, err := ioutil.TempDir("", "storagemarket_test")
	assert.NoError(t, err)
	ps := piecestore.NewPieceStore(td.Ds2)
	providerNode := testnodes.FakeProviderNode{
		FakeCommonNode: nodeCommon,
		MinerAddr:      providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	dt1 := graphsync.NewGraphSyncDataTransfer(td.Host1, td.GraphSync1)
	require.NoError(t, dt1.RegisterVoucherType(reflect.TypeOf(&requestvalidation.StorageDataTransferVoucher{}), &fakeDTValidator{}))

	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1),
		td.Bs1,
		dt1,
		discovery.NewLocal(td.Ds1),
		td.Ds1,
		&clientNode,
	)
	require.NoError(t, err)
	dt2 := graphsync.NewGraphSyncDataTransfer(td.Host2, td.GraphSync2)
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2),
		td.Ds1,
		td.Bs2,
		fs,
		ps,
		dt2,
		&providerNode,
		providerAddr,
		abi.RegisteredProof_StackedDRG2KiBPoSt,
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.AddAsk(big.NewInt(0), 50_000)
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

	var proposalCid cid.Cid

	// make a deal
	client.Run(ctx)
	dataRef := &storagemarket.DataRef{
		TransferType: storagemarket.TTManual,
		Root:         payloadCid,
	}

	carBuf := new(bytes.Buffer)

	err = cario.NewCarIO().WriteCar(ctx, td.Bs1, payloadCid, td.AllSelector, carBuf)
	require.NoError(t, err)

	commP, size, err := pieceio.GeneratePieceCommitment(abi.RegisteredProof_StackedDRG2KiBPoSt, carBuf, uint64(carBuf.Len()))

	assert.NoError(t, err)

	dataRef.PieceCid = &commP
	dataRef.PieceSize = size

	result, err := client.ProposeStorageDeal(ctx, providerAddr, &providerInfo, dataRef, abi.ChainEpoch(epoch+100), abi.ChainEpoch(epoch+20100), big.NewInt(1), big.NewInt(0), abi.RegisteredProof_StackedDRG2KiBPoSt)
	assert.NoError(t, err)

	proposalCid = result.ProposalCid

	time.Sleep(time.Millisecond * 100)

	cd, err := client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	assert.Equal(t, cd.State, storagemarket.StorageDealValidating)

	providerDeals, err := provider.ListLocalDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	assert.Equal(t, pd.State, storagemarket.StorageDealWaitingForData)

	err = cario.NewCarIO().WriteCar(ctx, td.Bs1, payloadCid, td.AllSelector, carBuf)
	require.NoError(t, err)
	err = provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	cd, err = client.GetLocalDeal(ctx, proposalCid)
	assert.NoError(t, err)
	assert.Equal(t, cd.State, storagemarket.StorageDealActive)

	providerDeals, err = provider.ListLocalDeals()
	assert.NoError(t, err)

	pd = providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	assert.Equal(t, pd.State, storagemarket.StorageDealCompleted)
}

type fakeDTValidator struct{}

func (v *fakeDTValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	return nil
}

func (v *fakeDTValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	return nil
}

var _ datatransfer.RequestValidator = (*fakeDTValidator)(nil)
