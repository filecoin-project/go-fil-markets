package shared_testutil

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"
)

func GenCARV2(t *testing.T, carV1Path string) (root cid.Cid, carV2FilePath string) {
	fpath, err := filepath.Abs(filepath.Join(thisDir(t), "..", carV1Path))
	require.NoError(t, err)

	f, err := os.Open(fpath)
	require.NoError(t, err)
	defer f.Close()

	r, err := car.NewCarReader(f)
	require.NoError(t, err)

	tmp, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	ingester, err := blockstore.NewReadWrite(tmp.Name(), r.Header.Roots)
	require.NoError(t, err)

	for {
		b, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		require.NoError(t, ingester.Put(b))
	}

	require.NoError(t, ingester.Finalize())

	require.Len(t, r.Header.Roots, 1)
	return r.Header.Roots[0], tmp.Name()
}
