package blockunsealing_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/blockunsealing"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"
)

func TestNewLoaderWithUnsealing(t *testing.T) {
	ctx := context.Background()
	cio := cario.NewCarIO()
	testdata := tut.NewTestIPLDTree()
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	var carBuffer bytes.Buffer
	err := cio.WriteCar(ctx, testdata, testdata.RootNodeLnk.(cidlink.Link).Cid, allSelector, &carBuffer)
	require.NoError(t, err)
	carData := carBuffer.Bytes()

	setupBlockStore := func(t *testing.T) bstore.Blockstore {
		bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
		err = bs.Put(testdata.RootBlock)
		require.NoError(t, err)
		return bs
	}
	deal1 := piecestore.DealInfo{
		DealID:   rand.Uint64(),
		SectorID: rand.Uint64(),
		Offset:   rand.Uint64(),
		Length:   rand.Uint64(),
	}
	deal2 := piecestore.DealInfo{
		DealID:   rand.Uint64(),
		SectorID: rand.Uint64(),
		Offset:   rand.Uint64(),
		Length:   rand.Uint64(),
	}
	piece := piecestore.PieceInfo{
		Deals: []piecestore.DealInfo{
			deal1,
			deal2,
		},
	}

	checkSuccessLoad := func(t *testing.T, loaderWithUnsealing blockunsealing.LoaderWithUnsealing, lnk ipld.Link) {
		read, err := loaderWithUnsealing.Load(lnk, ipld.LinkContext{})
		require.NoError(t, err)
		readData, err := ioutil.ReadAll(read)
		require.NoError(t, err)
		c, err := lnk.(cidlink.Link).Prefix().Sum(readData)
		require.Equal(t, c.Bytes(), lnk.(cidlink.Link).Bytes())
	}

	t.Run("when intermediate blockstore has block", func(t *testing.T) {
		bs := setupBlockStore(t)
		unsealer := testnodes.NewTestRetrievalProviderNode()
		loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
		checkSuccessLoad(t, loaderWithUnsealing, testdata.RootNodeLnk)
		unsealer.VerifyExpectations(t)
	})

	t.Run("when intermediate blockstore does not have block", func(t *testing.T) {
		t.Run("unsealing success on first ref", func(t *testing.T) {
			bs := setupBlockStore(t)
			unsealer := testnodes.NewTestRetrievalProviderNode()
			unsealer.ExpectUnseal(deal1.SectorID, deal1.Offset, deal1.Length, carData)
			loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
			checkSuccessLoad(t, loaderWithUnsealing, testdata.MiddleMapNodeLnk)
			unsealer.VerifyExpectations(t)
		})

		t.Run("unsealing success on later ref", func(t *testing.T) {
			bs := setupBlockStore(t)
			unsealer := testnodes.NewTestRetrievalProviderNode()
			unsealer.ExpectFailedUnseal(deal1.SectorID, deal1.Offset, deal1.Length)
			unsealer.ExpectUnseal(deal2.SectorID, deal2.Offset, deal2.Length, carData)
			loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
			checkSuccessLoad(t, loaderWithUnsealing, testdata.MiddleMapNodeLnk)
			unsealer.VerifyExpectations(t)
		})

		t.Run("fails all unsealing", func(t *testing.T) {
			bs := setupBlockStore(t)
			unsealer := testnodes.NewTestRetrievalProviderNode()
			unsealer.ExpectFailedUnseal(deal1.SectorID, deal1.Offset, deal1.Length)
			unsealer.ExpectFailedUnseal(deal2.SectorID, deal2.Offset, deal2.Length)
			loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
			_, err := loaderWithUnsealing.Load(testdata.MiddleMapNodeLnk, ipld.LinkContext{})
			require.Error(t, err)
			unsealer.VerifyExpectations(t)
		})

		t.Run("car io failure", func(t *testing.T) {
			bs := setupBlockStore(t)
			unsealer := testnodes.NewTestRetrievalProviderNode()
			randBytes := make([]byte, 100)
			_, err := rand.Read(randBytes)
			require.NoError(t, err)
			unsealer.ExpectUnseal(deal1.SectorID, deal1.Offset, deal1.Length, randBytes)
			loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
			_, err = loaderWithUnsealing.Load(testdata.MiddleMapNodeLnk, ipld.LinkContext{})
			require.Error(t, err)
			unsealer.VerifyExpectations(t)
		})

		t.Run("when piece was already unsealed", func(t *testing.T) {
			bs := setupBlockStore(t)
			unsealer := testnodes.NewTestRetrievalProviderNode()
			unsealer.ExpectUnseal(deal1.SectorID, deal1.Offset, deal1.Length, carData)
			loaderWithUnsealing := blockunsealing.NewLoaderWithUnsealing(ctx, bs, piece, cio, unsealer.UnsealSector)
			checkSuccessLoad(t, loaderWithUnsealing, testdata.MiddleMapNodeLnk)
			// clear out block store
			bs.DeleteBlock(testdata.MiddleMapBlock.Cid())

			// attemp to load again, will not unseal, will fail
			_, err := loaderWithUnsealing.Load(testdata.MiddleMapNodeLnk, ipld.LinkContext{})
			require.Error(t, err)

			unsealer.VerifyExpectations(t)
		})
	})
}
