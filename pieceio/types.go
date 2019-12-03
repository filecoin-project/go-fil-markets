package pieceio

import (
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
	GeneratePieceCommitment(bs ReadStore, payloadCid cid.Cid, selector ipld.Node) ([]byte, error)
	WritePayload(bs ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) ([]byte, error)
	ReadPiece(r io.Reader, bs WriteStore) (cid.Cid, error)
}
