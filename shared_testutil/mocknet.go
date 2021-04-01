package shared_testutil

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunk "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	"github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	dtnet "github.com/filecoin-project/go-data-transfer/network"
	"github.com/filecoin-project/go-multistore"
)

type Libp2pTestData struct {
	Ctx         context.Context
	Ds1         datastore.Batching
	Ds2         datastore.Batching
	Bs1         bstore.Blockstore
	Bs2         bstore.Blockstore
	MultiStore1 *multistore.MultiStore
	MultiStore2 *multistore.MultiStore
	DagService1 ipldformat.DAGService
	DagService2 ipldformat.DAGService
	DTNet1      dtnet.DataTransferNetwork
	DTNet2      dtnet.DataTransferNetwork
	DTStore1    datastore.Batching
	DTStore2    datastore.Batching
	DTTmpDir1   string
	DTTmpDir2   string
	Loader1     ipld.Loader
	Loader2     ipld.Loader
	Storer1     ipld.Storer
	Storer2     ipld.Storer
	Host1       host.Host
	Host2       host.Host
	OrigBytes   []byte

	MockNet mocknet.Mocknet
}

func NewLibp2pTestData(ctx context.Context, t *testing.T) *Libp2pTestData {
	testData := &Libp2pTestData{}
	testData.Ctx = ctx
	makeLoader := func(bs bstore.Blockstore) ipld.Loader {
		return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
			c, ok := lnk.(cidlink.Link)
			if !ok {
				return nil, errors.New("incorrect Link Type")
			}
			// read block from one store
			block, err := bs.Get(c.Cid)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(block.RawData()), nil
		}
	}

	makeStorer := func(bs bstore.Blockstore) ipld.Storer {
		return func(lnkCtx ipld.LinkContext) (io.Writer, ipld.StoreCommitter, error) {
			var buf bytes.Buffer
			var committer ipld.StoreCommitter = func(lnk ipld.Link) error {
				c, ok := lnk.(cidlink.Link)
				if !ok {
					return errors.New("incorrect Link Type")
				}
				block, err := blocks.NewBlockWithCid(buf.Bytes(), c.Cid)
				if err != nil {
					return err
				}
				return bs.Put(block)
			}
			return &buf, committer, nil
		}
	}
	var err error

	testData.Ds1 = dss.MutexWrap(datastore.NewMapDatastore())
	testData.Ds2 = dss.MutexWrap(datastore.NewMapDatastore())

	// make a bstore and dag service
	testData.Bs1 = bstore.NewBlockstore(testData.Ds1)
	testData.Bs2 = bstore.NewBlockstore(testData.Ds2)

	testData.MultiStore1, err = multistore.NewMultiDstore(testData.Ds1)
	require.NoError(t, err)
	testData.MultiStore2, err = multistore.NewMultiDstore(testData.Ds2)
	require.NoError(t, err)

	testData.DagService1 = merkledag.NewDAGService(blockservice.New(testData.Bs1, offline.Exchange(testData.Bs1)))
	testData.DagService2 = merkledag.NewDAGService(blockservice.New(testData.Bs2, offline.Exchange(testData.Bs2)))

	// setup an IPLD loader/storer for bstore 1
	testData.Loader1 = makeLoader(testData.Bs1)
	testData.Storer1 = makeStorer(testData.Bs1)

	// setup an IPLD loader/storer for bstore 2
	testData.Loader2 = makeLoader(testData.Bs2)
	testData.Storer2 = makeStorer(testData.Bs2)

	mn := mocknet.New(ctx)

	// setup network
	testData.Host1, err = mn.GenPeer()
	require.NoError(t, err)

	testData.Host2, err = mn.GenPeer()
	require.NoError(t, err)

	err = mn.LinkAll()
	require.NoError(t, err)

	testData.DTNet1 = dtnet.NewFromLibp2pHost(testData.Host1)
	testData.DTNet2 = dtnet.NewFromLibp2pHost(testData.Host2)

	testData.DTStore1 = namespace.Wrap(testData.Ds1, datastore.NewKey("DataTransfer1"))
	testData.DTStore2 = namespace.Wrap(testData.Ds1, datastore.NewKey("DataTransfer2"))

	testData.DTTmpDir1, err = ioutil.TempDir("", "dt-tmp-1")
	require.NoError(t, err)
	testData.DTTmpDir2, err = ioutil.TempDir("", "dt-tmp-2")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(testData.DTTmpDir1)
		_ = os.RemoveAll(testData.DTTmpDir2)
	})

	testData.MockNet = mn

	return testData
}

const unixfsChunkSize uint64 = 1 << 10
const unixfsLinksPerLevel = 1024

// LoadUnixFSFile injects the fixture `filename` into the given blockstore from the
// fixtures directory. If useSecondNode is true, fixture is injected to the second node;
// otherwise the first node gets it
func (ltd *Libp2pTestData) LoadUnixFSFile(t *testing.T, fixturesPath string, useSecondNode bool) ipld.Link {
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = ltd.DagService2
	} else {
		dagService = ltd.DagService1
	}
	return ltd.loadUnixFSFile(t, fixturesPath, dagService)
}

// LoadUnixFSFileToStore injects the fixture `filename` from the
// fixtures directory, creating a new multistore in the process. If useSecondNode is true,
// fixture is injected to the second node. Otherwise the first node gets it
func (ltd *Libp2pTestData) LoadUnixFSFileToStore(t *testing.T, fixturesPath string, useSecondNode bool) (ipld.Link, multistore.StoreID) {
	var storeID multistore.StoreID
	var dagService ipldformat.DAGService
	if useSecondNode {
		storeID = ltd.MultiStore2.Next()
		store, err := ltd.MultiStore2.Get(storeID)
		require.NoError(t, err)
		dagService = store.DAG
	} else {
		storeID = ltd.MultiStore1.Next()
		store, err := ltd.MultiStore1.Get(storeID)
		require.NoError(t, err)
		dagService = store.DAG
	}
	link := ltd.loadUnixFSFile(t, fixturesPath, dagService)
	return link, storeID
}

func (ltd *Libp2pTestData) loadUnixFSFile(t *testing.T, fixturesPath string, dagService ipldformat.DAGService) ipld.Link {

	// read in a fixture file
	fpath, err := filepath.Abs(filepath.Join(thisDir(t), "..", fixturesPath))
	require.NoError(t, err)

	f, err := os.Open(fpath)
	require.NoError(t, err)

	var buf bytes.Buffer
	tr := io.TeeReader(f, &buf)
	file := files.NewReaderFile(tr)

	// import to UnixFS
	bufferedDS := ipldformat.NewBufferedDAG(ltd.Ctx, dagService)

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

	// save the original files bytes
	ltd.OrigBytes = buf.Bytes()

	return cidlink.Link{Cid: nd.Cid()}
}

func thisDir(t *testing.T) string {
	_, fname, _, ok := runtime.Caller(1)
	require.True(t, ok)
	return path.Dir(fname)
}

// VerifyFileTransferred checks that the fixture file was sent from one node to the other.
func (ltd *Libp2pTestData) VerifyFileTransferred(t *testing.T, link ipld.Link, useSecondNode bool, readLen uint64) {
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = ltd.DagService2
	} else {
		dagService = ltd.DagService1
	}
	ltd.verifyFileTransferred(t, link, dagService, readLen)
}

// VerifyFileTransferredIntoStore checks that the fixture file was sent from one node to the other, into the store specified by
// storeID
func (ltd *Libp2pTestData) VerifyFileTransferredIntoStore(t *testing.T, link ipld.Link, storeID multistore.StoreID, useSecondNode bool, readLen uint64) {
	var dagService ipldformat.DAGService
	if useSecondNode {
		store, err := ltd.MultiStore2.Get(storeID)
		require.NoError(t, err)
		dagService = store.DAG
	} else {
		store, err := ltd.MultiStore1.Get(storeID)
		require.NoError(t, err)
		dagService = store.DAG
	}
	ltd.verifyFileTransferred(t, link, dagService, readLen)
}

func (ltd *Libp2pTestData) verifyFileTransferred(t *testing.T, link ipld.Link, dagService ipldformat.DAGService, readLen uint64) {

	c := link.(cidlink.Link).Cid

	// load the root of the UnixFS DAG from the new blockstore
	otherNode, err := dagService.Get(ltd.Ctx, c)
	require.NoError(t, err)

	// Setup a UnixFS file reader
	n, err := unixfile.NewUnixfsFile(ltd.Ctx, dagService, otherNode)
	require.NoError(t, err)

	fn, ok := n.(files.File)
	require.True(t, ok)

	// Read the bytes for the UnixFS File
	finalBytes := make([]byte, readLen)
	_, err = fn.Read(finalBytes)
	if err != nil {
		require.Equal(t, "EOF", err.Error())
	}

	// verify original bytes match final bytes!
	require.EqualValues(t, ltd.OrigBytes[:readLen], finalBytes)
}
