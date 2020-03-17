package blockrecorder_test

import (
	"bytes"
	"context"
	"testing"

	format "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	"github.com/ipld/go-car"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/blockrecorder"
)

func TestBlockRecording(t *testing.T) {

	sourceBserv := dstest.Bserv()
	sourceBs := sourceBserv.Blockstore()

	dserv := dag.NewDAGService(sourceBserv)
	a := dag.NewRawNode([]byte("aaaa"))
	b := dag.NewRawNode([]byte("bbbb"))
	c := dag.NewRawNode([]byte("cccc"))

	nd1 := &dag.ProtoNode{}
	_ = nd1.AddNodeLink("cat", a)

	nd2 := &dag.ProtoNode{}
	_ = nd2.AddNodeLink("first", nd1)
	_ = nd2.AddNodeLink("dog", b)

	nd3 := &dag.ProtoNode{}
	_ = nd3.AddNodeLink("second", nd2)
	_ = nd3.AddNodeLink("bear", c)

	ctx := context.Background()
	_ = dserv.Add(ctx, a)
	_ = dserv.Add(ctx, b)
	_ = dserv.Add(ctx, c)
	_ = dserv.Add(ctx, nd1)
	_ = dserv.Add(ctx, nd2)
	_ = dserv.Add(ctx, nd3)

	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	sc := car.NewSelectiveCar(ctx, sourceBs, []car.Dag{
		car.Dag{
			Root:     nd3.Cid(),
			Selector: node,
		},
	})

	carBuf := new(bytes.Buffer)
	blockLocationBuf := new(bytes.Buffer)
	sc.Write(carBuf, blockrecorder.RecordEachBlockTo(blockLocationBuf))

	metadata, err := blockrecorder.ReadBlockMetadata(blockLocationBuf)
	require.NoError(t, err)

	nds := []format.Node{
		a, b, nd1, nd2, nd3,
	}
	carBytes := carBuf.Bytes()
	for _, nd := range nds {
		cid := nd.Cid()
		var found bool
		var metadatum blockrecorder.PieceBlockMetadata
		for _, testMetadatum := range metadata {
			if testMetadatum.CID.Equals(cid) {
				metadatum = testMetadatum
				found = true
				break
			}
		}
		require.True(t, found)
		testBuf := carBytes[metadatum.Offset : metadatum.Offset+metadatum.Size]
		require.Equal(t, nd.RawData(), testBuf)
	}
}
