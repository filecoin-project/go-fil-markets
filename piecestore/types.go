package piecestore

import "github.com/ipfs/go-cid"

//go:generate cbor-gen-for PieceInfo DealInfo BlockInfo

// DealInfo is information about a single deal for a give piece
type DealInfo struct {
	DealID   uint64
	SectorID uint64
	Offset   uint64
	Length   uint64
}

// BlockInfo is information about where a given block is within a piece
type BlockInfo struct {
	CID       cid.Cid
	RelOffset uint64
	BlockSize uint64
}

// PieceInfo is metadata about a piece a provider may be storing based
// on its PieceCID -- so that, given a pieceCID during retrieval, the miner
// can determine how to unseal it if needed
type PieceInfo struct {
	PieceCID []byte
	Deals    []DealInfo
	Blocks   []BlockInfo
}

// PieceStore is a saved database of piece info that can be modified and queried
type PieceStore interface {
	AddDealForPiece(pieceCID []byte, dealInfo DealInfo) error
	AddBlockInfosToPiece(pieceCID []byte, blockInfos []BlockInfo) error
	HasBlockInfo(pieceCID []byte) (bool, error)
	HasDealInfo(pieceCID []byte) (bool, error)
	GetPieceInfo(pieceCID []byte) (PieceInfo, error)
}
