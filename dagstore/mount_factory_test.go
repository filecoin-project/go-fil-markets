package dagstore

import (
	"fmt"
	"net/url"
	"testing"

	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/stretchr/testify/require"
)

func TestLotusMountFactory(t *testing.T) {
	api := &lotusMountApiImpl{}
	l, err := NewLotusMountFactory(api)
	require.NoError(t, err)

	bgen := blocksutil.NewBlockGenerator()
	cid := bgen.Next().Cid()

	// success
	us := fmt.Sprintf(lotusMountURL, lotusScheme, cid.String())
	u, err := url.Parse(us)
	require.NoError(t, err)

	mnt, err := l.Parse(u)
	require.NoError(t, err)

	lm, ok := mnt.(*LotusMount)
	require.True(t, ok)
	require.Equal(t, cid, lm.PieceCid)
	require.Equal(t, u, lm.URL)
	require.Equal(t, api, lm.Api)

	// fails if scheme is not Lotus
	us = fmt.Sprintf(lotusMountURL, "http", cid.String())
	u, err = url.Parse(us)
	require.NoError(t, err)

	mnt, err = l.Parse(u)
	require.Error(t, err)
	require.Nil(t, mnt)
	require.Contains(t, err.Error(), "does not match")

	// fails if cid is not valid
	us = fmt.Sprintf(lotusMountURL, lotusScheme, "rand")
	u, err = url.Parse(us)
	require.NoError(t, err)
	mnt, err = l.Parse(u)
	require.Error(t, err)
	require.Nil(t, mnt)
	require.Contains(t, err.Error(), "failed to parse PieceCid")
}
