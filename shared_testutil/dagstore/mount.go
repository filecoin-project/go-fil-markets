package dagstore

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type MountApi interface {
	FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error)
}

type mockMount struct {
	pieceStore piecestore.PieceStore
	rm         retrievalmarket.RetrievalProviderNode
}

var _ MountApi = (*mockMount)(nil)

func NewFSMount(store piecestore.PieceStore, rm retrievalmarket.RetrievalProviderNode) *mockMount {
	return &mockMount{
		pieceStore: store,
		rm:         rm,
	}
}

func (l *mockMount) FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error) {
	pieceInfo, err := l.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch pieceInfo: %w", err)
	}

	if len(pieceInfo.Deals) <= 0 {
		return nil, xerrors.New("no storage deals for Piece")
	}

	// prefer an unsealed sector containing the piece if one exists
	for _, deal := range pieceInfo.Deals {
		isUnsealed, err := l.rm.IsUnsealed(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err != nil {
			continue
		}
		if isUnsealed {
			// UnsealSector will NOT unseal a sector if we already have an unsealed copy lying around.
			reader, err := l.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
			if err == nil {
				return reader, nil
			}
		}
	}

	lastErr := xerrors.New("no sectors found to unseal from")
	// if there is no unsealed sector containing the piece, just read the piece from the first sector we are able to unseal.
	for _, deal := range pieceInfo.Deals {
		reader, err := l.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err == nil {
			return reader, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
