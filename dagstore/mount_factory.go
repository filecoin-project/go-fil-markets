package dagstore

import (
	"context"
	"io"
	"net/url"

	"github.com/filecoin-project/dagstore/mount"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

var _ mount.MountFactory = (*LotusMountFactory)(nil)

type LotusMountAPI interface {
	FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error)
	GetUnpaddedCARSize(pieceCid cid.Cid) (uint64, error)
}

type LotusMountFactory struct {
	Api LotusMountAPI
}

func NewLotusMountFactory(api LotusMountAPI) (*LotusMountFactory, error) {
	return &LotusMountFactory{
		Api: api,
	}, nil
}

// Parse parses the shard specific state from the URL and returns a Mount for
//  the Shard represented by the URL.
func (l *LotusMountFactory) Parse(u *url.URL) (mount.Mount, error) {
	if u.Scheme != lotusScheme {
		return nil, xerrors.New("scheme does not match")
	}

	pieceCid, err := cid.Decode(u.Host)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse PieceCid from host, err=%s", err)
	}

	return &LotusMount{
		PieceCid: pieceCid,
		Api:      l.Api,
		URL:      u,
	}, nil
}
