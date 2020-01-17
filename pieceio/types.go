package pieceio

import (
	"github.com/filecoin-project/go-fil-markets/filestore"
	"io"

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
	GeneratePieceCommitment(payloadCid cid.Cid, selector ipld.Node) ([]byte, filestore.File, uint64, error)
	ReadPiece(r io.Reader) (cid.Cid, error)
}
