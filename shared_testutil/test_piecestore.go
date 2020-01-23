package shared_testutil

import (
	"errors"
	"testing"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/stretchr/testify/require"
)

// TestPieceStore is piecestore who's query results are mocked
type TestPieceStore struct {
	expectedPieces        map[string]piecestore.PieceInfo
	expectedMissingPieces map[string]struct{}
	receivedPieces        map[string]struct{}
	receivedMissingPieces map[string]struct{}
}

var _ piecestore.PieceStore = &TestPieceStore{}

// NewTestPieceStore creates a TestPieceStore
func NewTestPieceStore() *TestPieceStore {
	return &TestPieceStore{
		expectedPieces:        make(map[string]piecestore.PieceInfo),
		expectedMissingPieces: make(map[string]struct{}),
		receivedPieces:        make(map[string]struct{}),
		receivedMissingPieces: make(map[string]struct{}),
	}
}

// ExpectPiece records a piece being expected to be queried and return the given piece info
func (tps *TestPieceStore) ExpectPiece(pieceCid []byte, pieceInfo piecestore.PieceInfo) {
	tps.expectedPieces[string(pieceCid)] = pieceInfo
}

// ExpectMissingPiece records a piece being expected to be queried and should fail
func (tps *TestPieceStore) ExpectMissingPiece(pieceCid []byte) {
	tps.expectedMissingPieces[string(pieceCid)] = struct{}{}
}

// VerifyExpectations verifies that the piecestore was queried in the expected ways
func (tps *TestPieceStore) VerifyExpectations(t *testing.T) {
	require.Equal(t, len(tps.expectedPieces), len(tps.receivedPieces))
	require.Equal(t, len(tps.expectedMissingPieces), len(tps.receivedMissingPieces))
}

func (tps *TestPieceStore) AddDealForPiece(pieceCID []byte, dealInfo piecestore.DealInfo) error {
	panic("not implemented")
}

func (tps *TestPieceStore) AddBlockInfosToPiece(pieceCID []byte, blockInfos []piecestore.BlockInfo) error {
	panic("not implemented")
}

func (tps *TestPieceStore) HasBlockInfo(pieceCID []byte) (bool, error) {
	panic("not implemented")
}

func (tps *TestPieceStore) HasDealInfo(pieceCID []byte) (bool, error) {
	panic("not implemented")
}

func (tps *TestPieceStore) GetPieceInfo(pieceCID []byte) (piecestore.PieceInfo, error) {
	pio, ok := tps.expectedPieces[string(pieceCID)]
	if ok {
		tps.receivedPieces[string(pieceCID)] = struct{}{}
		return pio, nil
	}
	_, ok = tps.expectedMissingPieces[string(pieceCID)]
	if ok {
		tps.receivedMissingPieces[string(pieceCID)] = struct{}{}
		return piecestore.PieceInfoUndefined, retrievalmarket.ErrNotFound
	}
	return piecestore.PieceInfoUndefined, errors.New("GetPieceSize failed")
}
