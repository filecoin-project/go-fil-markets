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

var _ mktdagstore.DagStoreWrapper = (*MockDagStoreWrapper)(nil)

func NewMockDagStoreWrapper() *MockDagStoreWrapper {
	return &MockDagStoreWrapper{}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error {
	resch <- dagstore.ShardResult{}
	return nil
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	return nil, nil
}

func (m *MockDagStoreWrapper) Close() error {
	return nil
}
