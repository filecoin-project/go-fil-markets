package dagstore

import (
	"net/url"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore/mount"
)

var _ mount.Type = (*LotusMountFactory)(nil)

type LotusMountFactory struct {
	Api LotusMountAPI
}

func NewLotusMountFactory(api LotusMountAPI) (*LotusMountFactory, error) {
	return &LotusMountFactory{
		Api: api,
	}, nil
}

// Parse parses the shard specific state from the URL and returns a Lotus Mount for
// the Shard represented by the URL.
func (l *LotusMountFactory) Parse(u *url.URL) (mount.Mount, error) {
	if u.Scheme != lotusScheme {
		return nil, xerrors.New("scheme does not match")
	}

	pieceCid, err := cid.Decode(u.Host)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse PieceCid from host: %w", err)
	}

	return &LotusMount{
		PieceCid: pieceCid,
		Api:      l.Api,
		URL:      u,
	}, nil
}
