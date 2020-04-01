package blockio_test

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/blockio"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestSelectorReader(t *testing.T) {
	ctx := context.Background()
	testdata := tut.NewTestIPLDTree()

	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	sel := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	t.Run("reads correctly", func(t *testing.T) {
		reader := blockio.NewSelectorBlockReader(testdata.RootNodeLnk, sel, testdata.Loader)

		checkReadSequence(ctx, t, reader, []blocks.Block{
			testdata.RootBlock,
			testdata.LeafAlphaBlock,
			testdata.MiddleMapBlock,
			testdata.LeafAlphaBlock,
			testdata.MiddleListBlock,
			testdata.LeafAlphaBlock,
			testdata.LeafAlphaBlock,
			testdata.LeafBetaBlock,
			testdata.LeafAlphaBlock,
		})
	})

}

func checkReadSequence(ctx context.Context, t *testing.T, reader blockio.BlockReader, expectedBlks []blocks.Block) {
	for i := range expectedBlks {
		block, done, err := reader.ReadBlock(ctx)
		require.NoError(t, err)
		if i == len(expectedBlks)-1 {
			require.True(t, done)
		} else {
			require.False(t, done)
		}
		prefix, err := cid.PrefixFromBytes(block.Prefix)
		require.NoError(t, err)

		c, err := prefix.Sum(block.Data)
		require.NoError(t, err)

		require.Equal(t, c, expectedBlks[i].Cid())
	}
}
