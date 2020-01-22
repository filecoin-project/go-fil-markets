package piecestore_test

import (
	"crypto/sha256"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/piecestore"
)

func TestPieceStore(t *testing.T) {
	pieceCid := makeCid(t, "piece").Bytes()
	ps := piecestore.NewPieceStore(datastore.NewMapDatastore())

	// PieceInfo isn't being tracked yet
	_, err := ps.GetPieceInfo(pieceCid)
	assert.Error(t, err)

	// start tracking piece by adding a deal info
	err = ps.AddDealForPiece(pieceCid, piecestore.DealInfo{1, 2, 3, 256})
	assert.NoError(t, err)

	has, err := ps.HasDealInfo(pieceCid)
	assert.NoError(t, err)
	assert.True(t, has)

	pi, err := ps.GetPieceInfo(pieceCid)
	assert.NoError(t, err)
	assert.Equal(t, []piecestore.DealInfo{{1, 2, 3, 256}}, pi.Deals)

	// add another deal
	err = ps.AddDealForPiece(pieceCid, piecestore.DealInfo{5, 6, 7, 256})
	assert.NoError(t, err)

	has, err = ps.HasDealInfo(pieceCid)
	assert.NoError(t, err)
	assert.True(t, has)

	pi, err = ps.GetPieceInfo(pieceCid)
	assert.NoError(t, err)
	assert.Equal(t, []piecestore.DealInfo{{1, 2, 3, 256}, {5, 6, 7, 256}}, pi.Deals)

	// PieceInfo should have no block information
	has, err = ps.HasBlockInfo(pieceCid)
	assert.NoError(t, err)
	assert.False(t, has)

	// add block info
	blockCid := makeCid(t, "block0")
	blockInfos := []piecestore.BlockInfo{{blockCid, 42, 43}}

	err = ps.AddBlockInfosToPiece(pieceCid, blockInfos)
	assert.NoError(t, err)

	has, err = ps.HasBlockInfo(pieceCid)
	assert.NoError(t, err)
	assert.True(t, has)

	pi, err = ps.GetPieceInfo(pieceCid)
	assert.NoError(t, err)
	assert.Equal(t, []piecestore.BlockInfo{{blockCid, 42, 43}}, pi.Blocks)
}

func makeCid(t *testing.T, s string) cid.Cid {
	h := sha256.Sum256([]byte(s))
	mh, err := multihash.Encode(h[:], multihash.SHA2_256)
	assert.NoError(t, err)
	return cid.NewCidV1(cid.Raw, mh)
}
