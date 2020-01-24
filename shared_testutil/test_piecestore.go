package shared_testutil

import (
	"errors"
	"testing"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

// TestPieceStore is piecestore who's query results are mocked
type TestPieceStore struct {
	piecesStubbed    map[string]piecestore.PieceInfo
	piecesExpected   map[string]struct{}
	piecesReceived   map[string]struct{}
	cidInfosStubbed  map[cid.Cid]piecestore.CIDInfo
	cidInfosExpected map[cid.Cid]struct{}
	cidInfosReceived map[cid.Cid]struct{}
}

var _ piecestore.PieceStore = &TestPieceStore{}

// NewTestPieceStore creates a TestPieceStore
func NewTestPieceStore() *TestPieceStore {
	return &TestPieceStore{
		piecesStubbed:    make(map[string]piecestore.PieceInfo),
		piecesExpected:   make(map[string]struct{}),
		piecesReceived:   make(map[string]struct{}),
		cidInfosStubbed:  make(map[cid.Cid]piecestore.CIDInfo),
		cidInfosExpected: make(map[cid.Cid]struct{}),
		cidInfosReceived: make(map[cid.Cid]struct{}),
	}
}

// StubPiece creates a return value for the given piece cid without expecting it
// to be called
func (tps *TestPieceStore) StubPiece(pieceCid []byte, pieceInfo piecestore.PieceInfo) {
	tps.piecesStubbed[string(pieceCid)] = pieceInfo
}

// ExpectPiece records a piece being expected to be queried and return the given piece info
func (tps *TestPieceStore) ExpectPiece(pieceCid []byte, pieceInfo piecestore.PieceInfo) {
	tps.piecesExpected[string(pieceCid)] = struct{}{}
	tps.StubPiece(pieceCid, pieceInfo)
}

// ExpectMissingPiece records a piece being expected to be queried and should fail
func (tps *TestPieceStore) ExpectMissingPiece(pieceCid []byte) {
	tps.piecesExpected[string(pieceCid)] = struct{}{}
}

// StubCID creates a return value for the given CID without expecting it
// to be called
func (tps *TestPieceStore) StubCID(c cid.Cid, cidInfo piecestore.CIDInfo) {
	tps.cidInfosStubbed[c] = cidInfo
}

// ExpectCID records a CID being expected to be queried and return the given CID info
func (tps *TestPieceStore) ExpectCID(c cid.Cid, cidInfo piecestore.CIDInfo) {
	tps.cidInfosExpected[c] = struct{}{}
	tps.StubCID(c, cidInfo)
}

// ExpectMissingCID records a CID being expected to be queried and should fail
func (tps *TestPieceStore) ExpectMissingCID(c cid.Cid) {
	tps.cidInfosExpected[c] = struct{}{}
}

// VerifyExpectations verifies that the piecestore was queried in the expected ways
func (tps *TestPieceStore) VerifyExpectations(t *testing.T) {
	require.Equal(t, tps.piecesExpected, tps.piecesReceived)
	require.Equal(t, tps.cidInfosExpected, tps.cidInfosReceived)
}

func (tps *TestPieceStore) AddDealForPiece(pieceCID []byte, dealInfo piecestore.DealInfo) error {
	panic("not implemented")
}

func (tps *TestPieceStore) AddPieceBlockLocations(pieceCID []byte, blockLocations map[cid.Cid]piecestore.BlockLocation) error {
	panic("not implemented")
}

func (tps *TestPieceStore) GetPieceInfo(pieceCID []byte) (piecestore.PieceInfo, error) {
	tps.piecesReceived[string(pieceCID)] = struct{}{}

	pio, ok := tps.piecesStubbed[string(pieceCID)]
	if ok {
		return pio, nil
	}
	_, ok = tps.piecesExpected[string(pieceCID)]
	if ok {
		return piecestore.PieceInfoUndefined, retrievalmarket.ErrNotFound
	}
	return piecestore.PieceInfoUndefined, errors.New("GetPieceInfo failed")
}

func (tps *TestPieceStore) GetCIDInfo(c cid.Cid) (piecestore.CIDInfo, error) {
	tps.cidInfosReceived[c] = struct{}{}

	cio, ok := tps.cidInfosStubbed[c]
	if ok {
		return cio, nil
	}
	_, ok = tps.cidInfosExpected[c]
	if ok {
		return piecestore.CIDInfoUndefined, retrievalmarket.ErrNotFound
	}
	return piecestore.CIDInfoUndefined, errors.New("GetCIDInfo failed")
}
