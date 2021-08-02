package shared_testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
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
)

var defaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

// CreateDenseCARv2 generates a "dense" UnixFS CARv2 from the supplied ordinary file.
// A dense UnixFS CARv2 is one storing leaf data. Contrast to CreateRefCARv2.
func CreateDenseCARv2(t *testing.T, src string) (root cid.Cid, path string) {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root = buildUnixFS(t, src, dagSvc)

	// Create a UnixFS DAG again AND generate a CARv2 file using a CARv2
	// read-write blockstore now that we have the root.
	out, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, out.Close())

	rw, err := blockstore.OpenReadWrite(out.Name(), []cid.Cid{root}, blockstore.UseWholeCIDs(true))
	require.NoError(t, err)

	dagSvc = merkledag.NewDAGService(blockservice.New(rw, offline.Exchange(rw)))

	root2 := buildUnixFS(t, src, dagSvc)
	require.NoError(t, rw.Finalize())
	require.Equal(t, root, root2)

	return root, out.Name()
}

// CreateRefCARv2 generates a "ref" CARv2 from the supplied ordinary file.
// A "ref" CARv2 is one that stores leaf data as positional references to the original file.
func CreateRefCARv2(t *testing.T, src string) (cid.Cid, string) {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root := buildUnixFS(t, src, dagSvc)
	path := genRefCARv2(t, src, root)

	return root, path
}

func genRefCARv2(t *testing.T, fPath string, root cid.Cid) string {
	tmp, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	rw, err := blockstore.OpenReadWrite(tmp.Name(), []cid.Cid{root}, blockstore.UseWholeCIDs(true))
	require.NoError(t, err)

	fs, err := stores.FilestoreOf(rw)
	require.NoError(t, err)

	dagSvc := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))

	root2 := buildUnixFS(t, fPath, dagSvc)
	require.NoError(t, rw.Finalize())
	require.Equal(t, root, root2)

	// return the path of the CARv2 file.
	return tmp.Name()
}

func buildUnixFS(t *testing.T, from string, into ipldformat.DAGService) cid.Cid {
	// read in a fixture file
	fpath, err := filepath.Abs(filepath.Join(thisDir(t), "..", from))
	require.NoError(t, err)

	// open the fixture file
	f, err := os.Open(fpath)
	require.NoError(t, err)
	stat, err := f.Stat()
	require.NoError(t, err)

	// get a IPLD Reader Path File that can be used to read information
	// required to write the Unixfs DAG blocks to a filestore
	rpf, err := files.NewReaderPathFile(fpath, f, stat)
	require.NoError(t, err)

	// generate the dag and get the root
	// import to UnixFS
	prefix, err := merkledag.PrefixForCidVersion(1)
	require.NoError(t, err)
	prefix.MhType = defaultHashFunction

	bufferedDS := ipldformat.NewBufferedDAG(context.Background(), into)
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
