package blockunsealing

import (
	"context"
	"io"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

type UnsealingFunc func(ctx context.Context, sectorId uint64, offset uint64, length uint64) (io.ReadCloser, error)

func NewBlockstoreWithUnsealing(bs blockstore.Blockstore, pieceInfo piecestore.PieceInfo, unsealer UnsealingFunc) blockstore.Blockstore {
	// TODO: Implement for real
	return bs
}
