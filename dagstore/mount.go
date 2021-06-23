package dagstore

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/filecoin-project/dagstore/mount"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

const lotusScheme = "lotus"
const lotusMountURL = "%s://%s"

var _ mount.Mount = (*LotusMount)(nil)

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

func (l *LotusMount) Fetch(ctx context.Context) (io.ReadCloser, error) {
	return l.Api.FetchUnsealedPiece(ctx, l.PieceCid)
}

func (l *LotusMount) Info() mount.Info {
	return mount.Info{
		Kind: mount.MountKindRemote,
		URL:  l.URL,
	}
}

func (l *LotusMount) Stat() (mount.Stat, error) {
	size, err := l.Api.GetUnpaddedCARSize(l.PieceCid)
	if err != nil {
		return mount.Stat{}, xerrors.Errorf("failed to fetch piece size, err=%s", err)
	}

	return mount.Stat{
		Exists: true, // TODO Mark false when storage deal expires,
		Size:   size,
	}, nil
}
