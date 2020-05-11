package storageimpl_test

import (
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/stretchr/testify/assert"

	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
)

func TestConfigure(t *testing.T) {
	p := &storageimpl.Provider{}

	assert.False(t, p.UniversalRetrievalEnabled())
	assert.Equal(t, abi.ChainEpoch(0), p.DealAcceptanceBuffer())

	p.Configure(
		storageimpl.EnableUniversalRetrieval(),
		storageimpl.DealAcceptanceBuffer(abi.ChainEpoch(123)),
	)

	assert.True(t, p.UniversalRetrievalEnabled())
	assert.Equal(t, abi.ChainEpoch(123), p.DealAcceptanceBuffer())
}

/*
func TestRestartProvider(t *testing.T) {
	ctx := context.Background()
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	fs := shared_testutil.NewTestFileStore(shared_testutil.TestFileStoreParams{})
	ps := shared_testutil.NewTestPieceStore()
	dt2 := graphsync.NewGraphSyncDataTransfer(td.Host2, td.GraphSync2, td.DTStoredCounter2)
	assert.NoError(t, dt2.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, &fakeDTValidator{}))

	p, err := storageimpl.NewProvider(
		td.GraphSync1,
		td.Ds1,
		td.Bs1,
		fs,
		ps,
	)
}
*/
