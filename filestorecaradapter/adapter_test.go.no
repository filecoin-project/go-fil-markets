package filestorecaradapter

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
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
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car/v2/blockstore"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

const unixfsChunkSize uint64 = 1 << 10
const unixfsLinksPerLevel = 1024

var defaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

func TestReadOnlyFilstoreWithPosInfoCARFile(t *testing.T) {
	ctx := context.Background()
	normalFilePath, origBytes := createFile(t, 10, 10485760)

	// write out a unixfs dag to an inmemory store to get the root.
	root := writeUnixfsDAGInmemory(t, ctx, normalFilePath)
	require.NotEqualValues(t, cid.Undef, root)

	// write out a unixfs dag to a file store backed by a CAR file.
	tmpCARv2, err := os.CreateTemp(t.TempDir(), "rand")
	require.NoError(t, err)
	require.NoError(t, tmpCARv2.Close())
	fs, err := NewReadWriteFileStore(tmpCARv2.Name(), []cid.Cid{root})
	require.NoError(t, err)
	dagSvc := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))
	root2 := writeUnixfsDAGTo(t, ctx, normalFilePath, dagSvc)
	require.NoError(t, fs.Close())
	require.Equal(t, root, root2)

	// it works if we use a Filestore backed by the given CAR file
	rofs, err := NewReadOnlyFileStore(tmpCARv2.Name())
	require.NoError(t, err)
	fbz, err := dagToNormalFile(t, ctx, root, rofs)
	require.NoError(t, err)
	require.NoError(t, rofs.Close())

	// assert contents are equal
	require.EqualValues(t, origBytes, fbz)
}

func TestReadOnlyFilestoreWithFullCARFile(t *testing.T) {
	ctx := context.Background()
	normalFilePath, origContent := createFile(t, 10, 10485760)

	// write out a unixfs dag to an inmemory store to get the root.
	root := writeUnixfsDAGInmemory(t, ctx, normalFilePath)
	require.NotEqualValues(t, cid.Undef, root)

	// write out a unixfs dag to a read-write CARv2 blockstore to get the full CARv2 file.
	tmpCARv2, err := os.CreateTemp(t.TempDir(), "rand")
	require.NoError(t, err)
	require.NoError(t, tmpCARv2.Close())
	rw, err := blockstore.OpenReadWrite(tmpCARv2.Name(), []cid.Cid{root}, blockstore.UseWholeCIDs(true))
	require.NoError(t, err)
	dagSvc := merkledag.NewDAGService(blockservice.New(rw, offline.Exchange(rw)))
	root2 := writeUnixfsDAGTo(t, ctx, normalFilePath, dagSvc)
	require.NoError(t, rw.Finalize())
	require.Equal(t, root, root2)

	// Open a read only filestore with the full CARv2 file
	fs, err := NewReadOnlyFileStore(tmpCARv2.Name())
	require.NoError(t, err)

	// write out the normal file using the Filestore and assert the contents match.
	finalBytes, err := dagToNormalFile(t, ctx, root, fs)
	require.NoError(t, err)
	require.NoError(t, fs.Close())

	require.EqualValues(t, origContent, finalBytes)
}

func dagToNormalFile(t *testing.T, ctx context.Context, root cid.Cid, bs bstore.Blockstore) ([]byte, error) {
	outputF, err := os.CreateTemp(t.TempDir(), "rand")
	if err != nil {
		return nil, err
	}

	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dag := merkledag.NewDAGService(bsvc)
	nd, err := dag.Get(ctx, root)
	if err != nil {
		return nil, err
	}

	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	if err != nil {
		return nil, err
	}
	if err := files.WriteTo(file, outputF.Name()); err != nil {
		return nil, err
	}

	if _, err = outputF.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	finalBytes, err := ioutil.ReadAll(outputF)
	if err != nil {
		return nil, err
	}

	if err := outputF.Close(); err != nil {
		return nil, err
	}

	return finalBytes, nil
}

func createFile(t *testing.T, rseed int64, size int64) (path string, contents []byte) {
	source := io.LimitReader(rand.New(rand.NewSource(rseed)), size)

	file, err := os.CreateTemp(t.TempDir(), "sourcefile.dat")
	require.NoError(t, err)

	n, err := io.Copy(file, source)
	require.NoError(t, err)
	require.EqualValues(t, n, size)

	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	bz, err := ioutil.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	return file.Name(), bz
}

func writeUnixfsDAGInmemory(t *testing.T, ctx context.Context, filePath string) cid.Cid {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
	root := writeUnixfsDAGTo(t, ctx, filePath, dagSvc)
	require.NotEqualValues(t, cid.Undef, root)
	return root
}

func writeUnixfsDAGTo(t *testing.T, ctx context.Context, filePath string, dag ipldformat.DAGService) cid.Cid {
	normalFile, err := os.Open(filePath)
	require.NoError(t, err)
	defer normalFile.Close()

	stat, err := normalFile.Stat()
	require.NoError(t, err)

	// get a IPLD Reader Path File that can be used to read information required to write the Unixfs DAG blocks to a filestore
	rpf, err := files.NewReaderPathFile(normalFile.Name(), normalFile, stat)
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
