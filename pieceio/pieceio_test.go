package pieceio_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	dag "github.com/ipfs/go-merkledag"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	pmocks "github.com/filecoin-project/go-fil-markets/pieceio/mocks"
)

func Test_ThereAndBackAgain(t *testing.T) {
	cio := cario.NewCarIO()

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	pio := pieceio.NewPieceIO(cio, bs, multiStore)
	require.NoError(t, err)

	storeID := multiStore.Next()
	mstore, err := multiStore.Get(storeID)
	require.NoError(t, err)
	dserv := mstore.DAG
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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	r, _, err, writeErrChan := pio.GeneratePieceReader(nd3.Cid(), node, &storeID)
	require.NoError(t, err)

	id, err := pio.ReadPiece(&storeID, r)
	require.NoError(t, err)
	require.Equal(t, nd3.Cid(), id)
	err = r.Close()
	require.NoError(t, err)
	err = <-writeErrChan
	require.NoError(t, err)
}

func Test_StoreRestoreMemoryBuffer(t *testing.T) {
	cio := cario.NewCarIO()

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	pio := pieceio.NewPieceIO(cio, bs, multiStore)

	storeID := multiStore.Next()
	mstore, err := multiStore.Get(storeID)
	require.NoError(t, err)
	dserv := mstore.DAG
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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	r, pieceSize, err, writeErrChan := pio.GeneratePieceReader(nd3.Cid(), node, &storeID)
	require.NoError(t, err)
	commitment, paddedSize, err := pio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
	require.NoError(t, err)
	require.NotEqual(t, commitment, cid.Undef)

	paddedReader, secondPaddedSize := padreader.New(r, pieceSize)
	require.Equal(t, paddedSize, secondPaddedSize)
	secondCommitment, err := pieceio.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, paddedReader, paddedSize)
	require.NoError(t, err)
	require.Equal(t, commitment, secondCommitment)
	require.NoError(t, r.Close())
	require.NoError(t, <-writeErrChan)
}

func Test_Failures(t *testing.T) {

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	storeID := multiStore.Next()
	mstore, err := multiStore.Get(storeID)
	require.NoError(t, err)
	dserv := mstore.DAG
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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	t.Run("prepare CAR fails", func(t *testing.T) {

		ciomock := pmocks.CarIO{}
		any := mock.Anything
		ciomock.On("PrepareCar", any, any, any, any).Return(nil, fmt.Errorf("failed to prepare car"))
		pio := pieceio.NewPieceIO(&ciomock, bs, multiStore)
		_, _, err := pio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
		require.Error(t, err)
	})
	t.Run("PreparedCard dump operation fails", func(t *testing.T) {
		preparedCarMock := pmocks.PreparedCar{}
		ciomock := pmocks.CarIO{}
		any := mock.Anything
		ciomock.On("PrepareCar", any, any, any, any).Return(&preparedCarMock, nil)
		preparedCarMock.On("Size").Return(uint64(1000))
		preparedCarMock.On("Dump", any).Return(fmt.Errorf("failed to write car"))
		pio := pieceio.NewPieceIO(&ciomock, bs, multiStore)
		_, _, err := pio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
		require.Error(t, err)
	})
}
