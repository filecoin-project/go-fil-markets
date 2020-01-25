package piecestore

import (
	"github.com/ipfs/go-cid"
)

//go:generate cbor-gen-for PieceInfo DealInfo BlockLocation PieceBlockLocation CIDInfo

// DealInfo is information about a single deal for a give piece
type DealInfo struct {
	DealID   uint64
	SectorID uint64
	Offset   uint64
	Length   uint64
}

// BlockLocation is information about where a given block is relative to the overall piece
type BlockLocation struct {
	RelOffset uint64
	BlockSize uint64
}

// PieceBlockLocation is block information along with the pieceCID of the piece the block
// is inside of
type PieceBlockLocation struct {
	BlockLocation
	PieceCID []byte
}

// CIDInfo is information about where a given CID will live inside a piece
type CIDInfo struct {
	CID                 cid.Cid
	PieceBlockLocations []PieceBlockLocation
}

// CIDInfoUndefined is cid info with no information
var CIDInfoUndefined = CIDInfo{}

// PieceInfo is metadata about a piece a provider may be storing based
// on its PieceCID -- so that, given a pieceCID during retrieval, the miner
// can determine how to unseal it if needed
type PieceInfo struct {
	PieceCID []byte
	Deals    []DealInfo
}

// PieceInfoUndefined is piece info with no information
var PieceInfoUndefined = PieceInfo{}

// PieceStore is a saved database of piece info that can be modified and queried
type PieceStore interface {
	AddDealForPiece(pieceCID []byte, dealInfo DealInfo) error
	AddPieceBlockLocations(pieceCID []byte, blockLocations map[cid.Cid]BlockLocation) error
	GetPieceInfo(pieceCID []byte) (PieceInfo, error)
	GetCIDInfo(payloadCID cid.Cid) (CIDInfo, error)
}
