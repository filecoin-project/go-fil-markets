package shared_testutil

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-fil-markets/carstore"
	"github.com/filecoin-project/go-fil-markets/dagstore"
)

type MockDagStoreWrapper struct {
}

func NewMockDagStoreWrapper() *MockDagStoreWrapper {
	return &MockDagStoreWrapper{}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string) error {
	return nil
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	return nil, nil
}

func (m *MockDagStoreWrapper) Close() error {
	return nil
}

var _ dagstore.DagStoreWrapper = (*MockDagStoreWrapper)(nil)
