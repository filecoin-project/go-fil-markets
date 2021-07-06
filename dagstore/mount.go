package dagstore

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore/mount"
)

const lotusScheme = "lotus"
const lotusMountURL = "%s://%s"

var _ mount.Mount = (*LotusMount)(nil)

// LotusMount is the Lotus implementation of a Sharded DAG Store Mount.
// A Filecoin Piece is treated as a Shard by this implementation.
type LotusMount struct {
	PieceCid cid.Cid
	Api      LotusMountAPI
	URL      *url.URL
}

func NewLotusMount(pieceCid cid.Cid, api LotusMountAPI) (*LotusMount, error) {
	u := fmt.Sprintf(lotusMountURL, lotusScheme, pieceCid.String())
	url, err := url.Parse(u)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse URL, err=%s", err)
	}

	return &LotusMount{
		PieceCid: pieceCid,
		Api:      api,
		URL:      url,
	}, nil
}

func (l *LotusMount) Fetch(ctx context.Context) (mount.Reader, error) {
	r, err := l.Api.FetchUnsealedPiece(ctx, l.PieceCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch unsealed piece: %w", err)
	}
	return &readCloser{r}, nil
}

func (l *LotusMount) Info() mount.Info {
	return mount.Info{
		Kind:             mount.KindRemote,
		URL:              l.URL,
		AccessSequential: true,
		AccessSeek:       false,
		AccessRandom:     false,
	}
}

func (l *LotusMount) Close() error {
	return nil
}

func (l *LotusMount) Stat(_ context.Context) (mount.Stat, error) {
	size, err := l.Api.GetUnpaddedCARSize(l.PieceCid)
	if err != nil {
		return mount.Stat{}, xerrors.Errorf("failed to fetch piece size, err=%s", err)
	}

	// TODO Mark false when storage deal expires.
	return mount.Stat{
		Exists: true,
		Size:   int64(size),
	}, nil
}

type readCloser struct {
	io.ReadCloser
}

func (r *readCloser) ReadAt(p []byte, off int64) (n int, err error) {
	panic("not implemented")
}

func (r *readCloser) Seek(offset int64, whence int) (int64, error) {
	panic("not implemented")
}
