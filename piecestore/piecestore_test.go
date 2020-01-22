package piecestore_test

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/piecestore"
)

func TestStorePieceInfo(t *testing.T) {
	ps := piecestore.NewPieceStore(datastore.NewMapDatastore())
	pieceCid := []byte{1, 2, 3, 4}

	_, err := ps.GetPieceInfo(pieceCid)
	assert.Error(t, err)

	// Add a PieceInfo and some state
	testCid, err := cid.Decode("bafzbeigai3eoy2ccc7ybwjfz5r3rdxqrinwi4rwytly24tdbh6yk7zslrm")
	assert.NoError(t, err)
	blockInfos := []piecestore.BlockInfo{{testCid, 42, 43}}

	err = ps.AddBlockInfosToPiece(pieceCid, blockInfos)
	assert.NoError(t, err)

	pi, err := ps.GetPieceInfo(pieceCid)
	assert.NoError(t, err)
	assert.Len(t, pi.Blocks, 1)
	assert.Equal(t, pi.Blocks[0], piecestore.BlockInfo{testCid, 42, 43})
}
