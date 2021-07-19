package storageimpl

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/dagstore/shard"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	mktdagstore "github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	testnodes2 "github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestShardRegistration(t *testing.T) {
	ps := tut.NewTestPieceStore()
	providerNode := testnodes.NewTestRetrievalProviderNode()
	mountApi := mktdagstore.NewLotusMountAPI(ps, providerNode)
	dagStore := newMockDagStore()
	failureCh := make(chan dagstore.ShardResult, 1)
	dagStoreWrapper, err := mktdagstore.NewDagStoreWrapperWithDeps(dagStore, mountApi, failureCh)
	require.NoError(t, err)

	ctx := context.Background()
	cids := tut.GenerateCids(4)
	pieceCidUnsealed := cids[0]
	pieceCidSealed := cids[1]
	pieceCidUnsealed2 := cids[2]
	pieceCidUnsealed3 := cids[3]

	sealedSector := abi.SectorNumber(1)
	unsealedSector := abi.SectorNumber(2)
	unsealedSector2 := abi.SectorNumber(3)
	unsealedSector3 := abi.SectorNumber(4)

	providerAddr, err := address.NewIDAddress(1)
	require.NoError(t, err)

	spn := &testnodes2.FakeProviderNode{
		Sealed: map[abi.SectorNumber]bool{
			sealedSector:    true,
			unsealedSector:  false,
			unsealedSector2: false,
			unsealedSector3: false,
		},
	}
	ps.ExpectPiece(pieceCidUnsealed, piecestore.PieceInfo{
		PieceCID: pieceCidUnsealed,
		Deals: []piecestore.DealInfo{
			{
				SectorID: unsealedSector,
			},
		},
	})

	ps.ExpectPiece(pieceCidSealed, piecestore.PieceInfo{
		PieceCID: pieceCidSealed,
		Deals: []piecestore.DealInfo{
			{
				SectorID: sealedSector,
			},
		},
	})

	shardReg := NewShardMigrator(providerAddr, t.TempDir(), dagStoreWrapper, ps, spn)

	deals := []storagemarket.MinerDeal{{
		// Should be registered
		State:        storagemarket.StorageDealSealing,
		SectorNumber: unsealedSector,
		Ref: &storagemarket.DataRef{
			PieceCid: &pieceCidUnsealed,
		},
		CARv2FilePath: "",
	}, {
		// Should be registered with lazy registration (because sector is sealed)
		State:        storagemarket.StorageDealSealing,
		SectorNumber: sealedSector,
		Ref: &storagemarket.DataRef{
			PieceCid: &pieceCidSealed,
		},
		CARv2FilePath: "",
	}, {
		// Should be ignored because deal is no longer active
		State:        storagemarket.StorageDealError,
		SectorNumber: unsealedSector2,
		Ref: &storagemarket.DataRef{
			PieceCid: &pieceCidUnsealed2,
		},
		CARv2FilePath: "",
	}, {
		// Should be ignored because deal is not yet sealing
		State:        storagemarket.StorageDealFundsReserved,
		SectorNumber: unsealedSector3,
		Ref: &storagemarket.DataRef{
			PieceCid: &pieceCidUnsealed3,
		},
		CARv2FilePath: "",
	}}
	err = shardReg.registerShards(ctx, deals)
	require.NoError(t, err)

	// Only the deals in the appropriate state should be registered
	require.Equal(t, 2, dagStore.lenRegistrations())

	// The deal in an unsealed sector should be initialized immediately
	opts1, has1 := dagStore.getRegistration(shard.KeyFromCID(pieceCidUnsealed))
	require.True(t, has1)
	require.False(t, opts1.LazyInitialization)

	// The deal in a sealed sector should be initialized lazily
	opts2, has2 := dagStore.getRegistration(shard.KeyFromCID(pieceCidSealed))
	require.True(t, has2)
	require.True(t, opts2.LazyInitialization)

	// Clear out all deal registrations
	dagStore.clearRegistrations()

	// Run register shard migration again
	err = shardReg.registerShards(ctx, deals)
	require.NoError(t, err)

	// Should not call RegisterShard again because it should detect that the
	// migration has already been run
	require.Equal(t, 0, dagStore.lenRegistrations())

	ps.VerifyExpectations(t)
}

type mockDagStore struct {
	lk            sync.Mutex
	registrations map[shard.Key]dagstore.RegisterOpts
}

var _ mktdagstore.DagStore = (*mockDagStore)(nil)

func newMockDagStore() *mockDagStore {
	return &mockDagStore{
		registrations: make(map[shard.Key]dagstore.RegisterOpts),
	}
}

func (m *mockDagStore) RegisterShard(ctx context.Context, key shard.Key, mnt mount.Mount, out chan dagstore.ShardResult, opts dagstore.RegisterOpts) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations[key] = opts
	return nil
}

func (m *mockDagStore) lenRegistrations() int {
	m.lk.Lock()
	defer m.lk.Unlock()

	return len(m.registrations)
}

func (m *mockDagStore) getRegistration(key shard.Key) (dagstore.RegisterOpts, bool) {
	m.lk.Lock()
	defer m.lk.Unlock()

	opts, ok := m.registrations[key]
	return opts, ok
}

func (m *mockDagStore) clearRegistrations() {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations = make(map[shard.Key]dagstore.RegisterOpts)
}

func (m *mockDagStore) AcquireShard(ctx context.Context, key shard.Key, out chan dagstore.ShardResult, _ dagstore.AcquireOpts) error {
	return nil
}

func (m *mockDagStore) RecoverShard(ctx context.Context, key shard.Key, out chan dagstore.ShardResult, _ dagstore.RecoverOpts) error {
	return nil
}

func (m *mockDagStore) GC(ctx context.Context) (map[shard.Key]error, error) {
	return nil, nil
}

func (m *mockDagStore) Close() error {
	return nil
}
