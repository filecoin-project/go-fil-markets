package pieceio

import (
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"

	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
)

type WriteStore interface {
	Put(blocks.Block) error
}

type ReadStore interface {
	Get(cid.Cid) (blocks.Block, error)
}

// PieceIO converts between payloads and pieces
type PieceIO interface {
	GeneratePieceReader(payloadCid cid.Cid, selector ipld.Node, storeID *multistore.StoreID, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (io.ReadCloser, uint64, error, <-chan error)
	GeneratePieceCommitment(rt abi.RegisteredSealProof, payloadCid cid.Cid, selector ipld.Node, storeID *multistore.StoreID, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (cid.Cid, abi.UnpaddedPieceSize, error)
	ReadPiece(storeID *multistore.StoreID, r io.Reader) (cid.Cid, error)
}
