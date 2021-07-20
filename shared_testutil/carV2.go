package shared_testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	cidutil "github.com/ipfs/go-cidutil"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunk "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car/v2/blockstore"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestorecaradapter"
)

var defaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

// GenFullCARv2FromNormalFile generates a CARv2 file from a "normal" i.e. non-CAR file and returns the file path.
// All the Unixfs blocks are written as is in the CARv2 file.
func GenFullCARv2FromNormalFile(t *testing.T, normalFilePath string) (root cid.Cid, carV2FilePath string) {
	ctx := context.Background()

	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root = genUnixfsDAG(t, ctx, normalFilePath, dagSvc)
	// Create a UnixFS DAG again AND generate a CARv2 file using a CARv2 read-write blockstore now that we have the root.
	carV2Path := genFullCARv2File(t, ctx, normalFilePath, root)

	return root, carV2Path
}

// GenFileStoreCARv2FromNormalFile generates a CARv2 file that can be used to back a Filestore from a "normal" i.e. non-CAR file and returns the file path.
func GenFileStoreCARv2FromNormalFile(t *testing.T, normalFilePath string) (root cid.Cid, carV2FilePath string) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
	root = genUnixfsDAG(t, ctx, normalFilePath, dagSvc)
	// Create a UnixFS DAG again AND generate a CARv2 file using a filestore backed by a CARv2 read-write blockstore.
	carV2Path := genFileStoreCARv2File(t, ctx, normalFilePath, root)

	return root, carV2Path
}

func genFullCARv2File(t *testing.T, ctx context.Context, fPath string, root cid.Cid) string {
	tmp, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	rw, err := blockstore.OpenReadWrite(tmp.Name(), []cid.Cid{root}, blockstore.UseWholeCIDs(true))
	require.NoError(t, err)
	dagSvc := merkledag.NewDAGService(blockservice.New(rw, offline.Exchange(rw)))

	root2 := genUnixfsDAG(t, ctx, fPath, dagSvc)
	require.NoError(t, rw.Finalize())
	require.Equal(t, root, root2)

	// return the path of the CARv2 file.
	return tmp.Name()
}

func genFileStoreCARv2File(t *testing.T, ctx context.Context, fPath string, root cid.Cid) string {
	tmp, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	fs, err := filestorecaradapter.NewReadWriteFileStore(tmp.Name(), []cid.Cid{root})
	require.NoError(t, err)

	dagSvc := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))

	root2 := genUnixfsDAG(t, ctx, fPath, dagSvc)
	require.NoError(t, fs.Close())
	require.Equal(t, root, root2)

	// return the path of the CARv2 file.
	return tmp.Name()
}

func genUnixfsDAG(t *testing.T, ctx context.Context, normalFilePath string, dag ipldformat.DAGService) cid.Cid {
	// read in a fixture file
	fpath, err := filepath.Abs(filepath.Join(thisDir(t), "..", normalFilePath))
	require.NoError(t, err)
	// open the fixture file
	f, err := os.Open(fpath)
	require.NoError(t, err)
	stat, err := f.Stat()
	require.NoError(t, err)

	// get a IPLD Reader Path File that can be used to read information required to write the Unixfs DAG blocks to a filestore
	rpf, err := files.NewReaderPathFile(fpath, f, stat)
	require.NoError(t, err)

	// generate the dag and get the root
	// import to UnixFS
	prefix, err := merkledag.PrefixForCidVersion(1)
	require.NoError(t, err)
	prefix.MhType = defaultHashFunction

	bufferedDS := ipldformat.NewBufferedDAG(ctx, dag)
	params := ihelper.DagBuilderParams{
		Maxlinks:  unixfsLinksPerLevel,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufferedDS,
		NoCopy:  true,
	}

	db, err := params.New(chunk.NewSizeSplitter(rpf, int64(unixfsChunkSize)))
	require.NoError(t, err)

	nd, err := balanced.Layout(db)
	require.NoError(t, err)

	err = bufferedDS.Commit()
	require.NoError(t, err)
	require.NoError(t, rpf.Close())

	return nd.Cid()
}
