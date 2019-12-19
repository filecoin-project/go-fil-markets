package shared_testutil

import (
	"bytes"
	"errors"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-graphsync/ipldbridge"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"io"
	"testing"
)

type Libp2pTestData struct {
	Ctx         context.Context
	Bs1         bstore.Blockstore
	Bs2         bstore.Blockstore
	DagService1 ipldformat.DAGService
	DagService2 ipldformat.DAGService
	Loader1     ipld.Loader
	Loader2     ipld.Loader
	Storer1     ipld.Storer
	Storer2     ipld.Storer
	Host1       host.Host
	Host2       host.Host
	Bridge1     ipldbridge.IPLDBridge
	Bridge2     ipldbridge.IPLDBridge
	AllSelector ipld.Node
	OrigBytes   []byte
}

func NewLibp2pTestData(ctx context.Context, t *testing.T) *Libp2pTestData {
	testData := &Libp2pTestData{}
	testData.Ctx = ctx
	makeLoader := func(bs bstore.Blockstore) ipld.Loader {
		return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
			c, ok := lnk.(cidlink.Link)
			if !ok {
				return nil, errors.New("Incorrect Link Type")
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
					return errors.New("Incorrect Link Type")
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
	// make a bstore and dag service
	testData.bs1 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	testData.bs2 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	testData.DagService1 = merkledag.NewDAGService(blockservice.New(testData.bs1, offline.Exchange(testData.bs1)))
	testData.DagService2 = merkledag.NewDAGService(blockservice.New(testData.bs2, offline.Exchange(testData.bs2)))

	// setup an IPLD loader/storer for bstore 1
	testData.Loader1 = makeLoader(testData.bs1)
	testData.Storer1 = makeStorer(testData.bs1)

	// setup an IPLD loader/storer for bstore 2
	testData.Loader2 = makeLoader(testData.bs2)
	testData.Storer2 = makeStorer(testData.bs2)

	mn := mocknet.New(ctx)

	// setup network
	var err error
	testData.Host1, err = mn.GenPeer()
	require.NoError(t, err)

	testData.Host2, err = mn.GenPeer()
	require.NoError(t, err)

	err = mn.LinkAll()
	require.NoError(t, err)

	testData.Bridge1 = ipldbridge.NewIPLDBridge()
	testData.Bridge2 = ipldbridge.NewIPLDBridge()

	// create a selector for the whole UnixFS dag
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	testData.AllSelector = ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	return testData
}
