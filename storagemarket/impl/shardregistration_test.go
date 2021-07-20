package storageimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	testnodes2 "github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestShardRegistration(t *testing.T) {
	ps := tut.NewTestPieceStore()
	providerNode := testnodes.NewTestRetrievalProviderNode()
	dagStoreWrapper := tut.NewMockDagStoreWrapper(ps, providerNode)

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
	require.Equal(t, 2, dagStoreWrapper.LenRegistrations())

	// The deal in an unsealed sector should be initialized immediately
	reg1, has1 := dagStoreWrapper.GetRegistration(pieceCidUnsealed)
	require.True(t, has1)
	require.True(t, reg1.EagerInit)

	// The deal in a sealed sector should be initialized lazily
	reg2, has2 := dagStoreWrapper.GetRegistration(pieceCidSealed)
	require.True(t, has2)
	require.False(t, reg2.EagerInit)

	// Clear out all deal registrations
	dagStoreWrapper.ClearRegistrations()

	// Run register shard migration again
	err = shardReg.registerShards(ctx, deals)
	require.NoError(t, err)

	// Should not call RegisterShard again because it should detect that the
	// migration has already been run
	require.Equal(t, 0, dagStoreWrapper.LenRegistrations())

	ps.VerifyExpectations(t)
}
