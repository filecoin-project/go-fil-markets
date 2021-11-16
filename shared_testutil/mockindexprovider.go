package shared_testutil

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"

	provider "github.com/filecoin-project/index-provider"
	stiapi "github.com/filecoin-project/storetheindex/api/v0"
)

type MockIndexProvider struct {
	provider.Interface

	lk       sync.Mutex
	callback provider.Callback
	notifs   map[string]stiapi.Metadata
}

func NewMockIndexProvider() *MockIndexProvider {
	return &MockIndexProvider{
		notifs: make(map[string]stiapi.Metadata),
	}

}

func (m *MockIndexProvider) RegisterCallback(cb provider.Callback) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.callback = cb
}

func (m *MockIndexProvider) NotifyPut(ctx context.Context, contextID []byte, metadata stiapi.Metadata) (cid.Cid, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.notifs[string(contextID)] = metadata

	return cid.Undef, nil
}

func (m *MockIndexProvider) GetNotifs() map[string]stiapi.Metadata {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.notifs
}
