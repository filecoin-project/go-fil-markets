package shared_testutil

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/carstore"
	mktdagstore "github.com/filecoin-project/go-fil-markets/dagstore"
)

type MockDagStoreWrapper struct {
}

func NewMockDagStoreWrapper() *MockDagStoreWrapper {
	return &MockDagStoreWrapper{}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool) error {
	return nil
}

func (m *MockDagStoreWrapper) RegisterShardAsync(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) {
	resch <- dagstore.ShardResult{}
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	return nil, nil
}

func (m *MockDagStoreWrapper) Close() error {
	return nil
}

var _ mktdagstore.DagStoreWrapper = (*MockDagStoreWrapper)(nil)
