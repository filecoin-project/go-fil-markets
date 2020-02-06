package storagemarket_test

import (
	"context"
	"io"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	graphsync "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

func TestMakeDeal(t *testing.T) {
	ctx := context.Background()
	epoch := uint64(100)
	nodeCommon := fakeCommon{newStorageMarketState()}
	ds1 := datastore.NewMapDatastore()
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	rootLink := td.LoadUnixFSFile(t, "payload.txt", false)
	payloadCid := rootLink.(cidlink.Link).Cid

	clientNode := fakeClientNode{
		fakeCommon: nodeCommon,
		ClientAddr: address.TestAddress,
	}

	providerAddr := address.TestAddress2
	ds2 := datastore.NewMapDatastore()
	tempPath, err := ioutil.TempDir("", "storagemarket_test")
	assert.NoError(t, err)
	ps := piecestore.NewPieceStore(ds2)
	providerNode := fakeProviderNode{
		fakeCommon: nodeCommon,
		MinerAddr:  providerAddr,
	}
	fs, err := filestore.NewLocalFileStore(filestore.OsPath(tempPath))
	assert.NoError(t, err)

	// create provider and client
	dt1 := graphsync.NewGraphSyncDataTransfer(td.Host1, td.GraphSync1)
	require.NoError(t, dt1.RegisterVoucherType(reflect.TypeOf(&storageimpl.StorageDataTransferVoucher{}), &fakeDTValidator{}))

	client := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1),
		td.Bs1,
		dt1,
		discovery.NewLocal(ds1),
		statestore.New(ds1),
		&clientNode,
	)

	dt2 := graphsync.NewGraphSyncDataTransfer(td.Host2, td.GraphSync2)
	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2),
		ds2,
		td.Bs2,
		fs,
		ps,
		dt2,
		&providerNode,
		providerAddr,
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
		SectorSize: 32,
		PeerID:     td.Host2.ID(),
	}

	var proposalCid cid.Cid

	// make a deal
	go func() {
		client.Run(ctx)
		dataRef := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         payloadCid,
		}
		result, err := client.ProposeStorageDeal(ctx, providerAddr, &providerInfo, dataRef, storagemarket.Epoch(epoch+100), 20000, big.NewInt(1), big.NewInt(0))
		assert.NoError(t, err)

		proposalCid = result.ProposalCid
	}()

	time.Sleep(time.Millisecond * 100)

	cd, err := client.GetInProgressDeal(ctx, proposalCid)
	assert.NoError(t, err)
	assert.Equal(t, cd.State, storagemarket.StorageDealActive)

	providerDeals, err := provider.ListIncompleteDeals()
	assert.NoError(t, err)

	pd := providerDeals[0]
	assert.True(t, pd.ProposalCid.Equals(proposalCid))
	assert.Equal(t, pd.State, storagemarket.StorageDealActive)
}

type fakeDTValidator struct{}

func (v *fakeDTValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	return nil
}

func (v *fakeDTValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	return nil
}

var _ datatransfer.RequestValidator = (*fakeDTValidator)(nil)

// Below fake node implementations
type testStateKey struct{ Epoch uint64 }

func (k *testStateKey) Height() uint64 {
	return k.Epoch
}

type storageMarketState struct {
	Epoch        uint64
	DealId       uint64
	Balances     map[address.Address]abi.TokenAmount
	StorageDeals map[address.Address][]storagemarket.StorageDeal
	Providers    []*storagemarket.StorageProviderInfo
}

func newStorageMarketState() *storageMarketState {
	return &storageMarketState{
		Epoch:        0,
		DealId:       0,
		Balances:     map[address.Address]abi.TokenAmount{},
		StorageDeals: map[address.Address][]storagemarket.StorageDeal{},
		Providers:    nil,
	}
}

func (sma *storageMarketState) AddFunds(addr address.Address, amount abi.TokenAmount) {
	if existing, ok := sma.Balances[addr]; ok {
		sma.Balances[addr] = big.Add(existing, amount)
	} else {
		sma.Balances[addr] = amount
	}
}

func (sma *storageMarketState) Balance(addr address.Address) storagemarket.Balance {
	if existing, ok := sma.Balances[addr]; ok {
		return storagemarket.Balance{big.NewInt(0), existing}
	}
	return storagemarket.Balance{big.NewInt(0), big.NewInt(0)}
}

func (sma *storageMarketState) Deals(addr address.Address) []storagemarket.StorageDeal {
	if existing, ok := sma.StorageDeals[addr]; ok {
		return existing
	}
	return nil
}

func (sma *storageMarketState) StateKey() storagemarket.StateKey {
	return &testStateKey{sma.Epoch}
}

func (sma *storageMarketState) AddDeal(deal storagemarket.StorageDeal) storagemarket.StateKey {
	for _, addr := range []address.Address{deal.Client, deal.Provider} {
		if existing, ok := sma.StorageDeals[addr]; ok {
			sma.StorageDeals[addr] = append(existing, deal)
		} else {
			sma.StorageDeals[addr] = []storagemarket.StorageDeal{deal}
		}
	}
	return sma.StateKey()
}

type fakeCommon struct {
	SMState *storageMarketState
}

func (n *fakeCommon) MostRecentStateId(ctx context.Context) (storagemarket.StateKey, error) {
	return n.SMState.StateKey(), nil
}

func (n *fakeCommon) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	n.SMState.AddFunds(addr, amount)
	return nil
}

func (n *fakeCommon) EnsureFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	balance := n.SMState.Balance(addr)
	if balance.Available.LessThan(amount) {
		n.SMState.AddFunds(addr, big.Sub(amount, balance.Available))
	}
	return nil
}

func (n *fakeCommon) GetBalance(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	return n.SMState.Balance(addr), nil
}

func (n *fakeCommon) VerifySignature(signature crypto.Signature, addr address.Address, data []byte) bool {
	return true
}

type fakeClientNode struct {
	fakeCommon
	ClientAddr      address.Address
	ValidationError error
}

func (n *fakeClientNode) ListClientDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	return n.SMState.Deals(addr), nil
}

func (n *fakeClientNode) ListStorageProviders(ctx context.Context) ([]*storagemarket.StorageProviderInfo, error) {
	return n.SMState.Providers, nil
}

func (n *fakeClientNode) ValidatePublishedDeal(ctx context.Context, deal storagemarket.ClientDeal) (uint64, error) {
	return 0, nil
}

func (n *fakeClientNode) SignProposal(ctx context.Context, signer address.Address, proposal *storagemarket.StorageDealProposal) error {
	proposal.ProposerSignature = shared_testutil.MakeTestSignature()
	return nil
}

func (n *fakeClientNode) GetDefaultWalletAddress(ctx context.Context) (address.Address, error) {
	return n.ClientAddr, nil
}

func (n *fakeClientNode) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealId uint64, cb storagemarket.DealSectorCommittedCallback) error {
	cb(nil)
	return nil
}

func (n *fakeClientNode) ValidateAskSignature(ask *storagemarket.SignedStorageAsk) error {
	return n.ValidationError
}

var _ storagemarket.StorageClientNode = (*fakeClientNode)(nil)

type fakeProviderNode struct {
	fakeCommon
	MinerAddr     address.Address
	Epoch         uint64
	PieceLength   uint64
	PieceSectorID uint64
	CompletedDeal storagemarket.MinerDeal
	PublishDealID storagemarket.DealID
}

func (n *fakeProviderNode) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (storagemarket.DealID, cid.Cid, error) {
	p := deal.Proposal

	sd := storagemarket.StorageDeal{
		PieceRef:             p.PieceRef,
		PieceSize:            p.PieceSize,
		Client:               p.Client,
		Provider:             p.Provider,
		ProposalExpiration:   p.ProposalExpiration,
		Duration:             p.Duration,
		StoragePricePerEpoch: p.StoragePricePerEpoch,
		StorageCollateral:    p.StorageCollateral,
	}

	n.SMState.AddDeal(sd)

	return n.PublishDealID, shared_testutil.GenerateCids(1)[0], nil
}

func (n *fakeProviderNode) ListProviderDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	return n.SMState.Deals(addr), nil
}

func (n *fakeProviderNode) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize uint64, pieceReader io.Reader) error {
	return nil
}

func (n *fakeProviderNode) GetMinerWorker(ctx context.Context, miner address.Address) (address.Address, error) {
	return n.MinerAddr, nil
}

func (n *fakeProviderNode) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	return shared_testutil.MakeTestSignature(), nil
}

func (n *fakeProviderNode) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID uint64, cb storagemarket.DealSectorCommittedCallback) error {
	cb(nil)
	return nil
}

func (n *fakeProviderNode) LocatePieceForDealWithinSector(ctx context.Context, dealID uint64) (sectorID uint64, offset uint64, length uint64, err error) {
	return n.PieceSectorID, 0, n.PieceLength, nil
}

var _ storagemarket.StorageProviderNode = (*fakeProviderNode)(nil)
