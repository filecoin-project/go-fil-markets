package pieceio

import (
	"io"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/specs-actors/actors/abi"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
)

type WriteStore interface {
	Put(blocks.Block) error
}

type ReadStore interface {
	Get(cid.Cid) (blocks.Block, error)
}

// PieceIO converts between payloads and pieces
type PieceIO interface {
	GeneratePieceCommitment(rt abi.RegisteredProof, payloadCid cid.Cid, selector ipld.Node) (cid.Cid, abi.UnpaddedPieceSize, error)
	ReadPiece(r io.Reader) (cid.Cid, error)
}

type PieceIOWithStore interface {
	PieceIO
	GeneratePieceCommitmentToFile(rt abi.RegisteredProof, payloadCid cid.Cid, selector ipld.Node) (cid.Cid, filestore.Path, abi.UnpaddedPieceSize, error)
}
