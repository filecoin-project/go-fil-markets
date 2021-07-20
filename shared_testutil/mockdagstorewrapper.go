package shared_testutil

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/ipfs/go-cid"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/carstore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

type registration struct {
	CarPath   string
	EagerInit bool
}

// MockDagStoreWrapper is used to mock out the DAG store wrapper operations
// for the tests.
// It simulates getting deal info from a piece store and unsealing the data for
// the deal from a retrieval provider node.
type MockDagStoreWrapper struct {
	pieceStore piecestore.PieceStore
	rpn        retrievalmarket.RetrievalProviderNode

	lk            sync.Mutex
	registrations map[cid.Cid]registration
}

var _ shared.DagStoreWrapper = (*MockDagStoreWrapper)(nil)

func NewMockDagStoreWrapper(pieceStore piecestore.PieceStore, rpn retrievalmarket.RetrievalProviderNode) *MockDagStoreWrapper {
	return &MockDagStoreWrapper{
		pieceStore:    pieceStore,
		rpn:           rpn,
		registrations: make(map[cid.Cid]registration),
	}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations[pieceCid] = registration{
		CarPath:   carPath,
		EagerInit: eagerInit,
	}

	resch <- dagstore.ShardResult{}
	return nil
}

func (m *MockDagStoreWrapper) LenRegistrations() int {
	m.lk.Lock()
	defer m.lk.Unlock()

	return len(m.registrations)
}

func (m *MockDagStoreWrapper) GetRegistration(pieceCid cid.Cid) (registration, bool) {
	m.lk.Lock()
	defer m.lk.Unlock()

	reg, ok := m.registrations[pieceCid]
	return reg, ok
}

func (m *MockDagStoreWrapper) ClearRegistrations() {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations = make(map[cid.Cid]registration)
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	_, ok := m.registrations[pieceCid]
	if !ok {
		return nil, xerrors.Errorf("no shard for piece CID %s", pieceCid)
	}

	// Get the piece info from the piece store
	pi, err := m.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, err
	}

	// Unseal the sector data for the deal
	deal := pi.Deals[0]
	r, err := m.rpn.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
	if err != nil {
		return nil, xerrors.Errorf("error unsealing deal for piece %s: %w", pieceCid, err)
	}

	return getBlockstoreFromReader(r, pieceCid)
}

func getBlockstoreFromReader(r io.ReadCloser, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	// Write the piece to a file
	tmpFile, err := os.CreateTemp("", "dagstoretmp")
	if err != nil {
		return nil, xerrors.Errorf("creating temp file for piece CID %s: %w", pieceCid, err)
	}

	_, err = io.Copy(tmpFile, r)
	if err != nil {
		return nil, xerrors.Errorf("copying read stream to temp file for piece CID %s: %w", pieceCid, err)
	}

	err = tmpFile.Close()
	if err != nil {
		return nil, xerrors.Errorf("closing temp file for piece CID %s: %w", pieceCid, err)
	}

	// Get a blockstore from the CAR file
	return blockstore.OpenReadOnly(tmpFile.Name(), carv2.ZeroLengthSectionAsEOF(true))
}

func (m *MockDagStoreWrapper) Close() error {
	return nil
}
