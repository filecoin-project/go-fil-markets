package shared_testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunk "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	"github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/stretchr/testify/require"
)

// GenCARv2FromNormalFile generates a CARv2 file from a "normal" i.e. non-CAR file and returns the file path.
func GenCARv2FromNormalFile(t *testing.T, normalFilePath string) (root cid.Cid, carV2FilePath string, blockstore bstore.Blockstore) {
	ctx := context.Background()
	// read in a fixture file
	fpath, err := filepath.Abs(filepath.Join(thisDir(t), "..", normalFilePath))
	require.NoError(t, err)
	f, err := os.Open(fpath)
	require.NoError(t, err)
	file := files.NewReaderFile(f)
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	// import to UnixFS
	bufferedDS := ipldformat.NewBufferedDAG(ctx, dag)

	params := helpers.DagBuilderParams{
		Maxlinks:   unixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS,
	}

	db, err := params.New(chunk.NewSizeSplitter(file, int64(unixfsChunkSize)))
	require.NoError(t, err)

	nd, err := balanced.Layout(db)
	require.NoError(t, err)

	err = bufferedDS.Commit()
	require.NoError(t, err)
	require.NoError(t, file.Close())

	// Create a UnixFS DAG again AND generate a CARv2 file using a CARv2 read-write blockstore now that we have the root.
	carV2Path := genWithCARv2Blockstore(t, fpath, nd.Cid())

	return nd.Cid(), carV2Path, bs
}
