package pieceio_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/filestore"
	fsmocks "github.com/filecoin-project/go-fil-markets/filestore/mocks"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	pmocks "github.com/filecoin-project/go-fil-markets/pieceio/mocks"
)

func Test_ThereAndBackAgain(t *testing.T) {
	tempDir := filestore.OsPath("./tempDir")
	cio := cario.NewCarIO()

	store, err := filestore.NewLocalFileStore(tempDir)
	require.NoError(t, err)

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	pio := pieceio.NewPieceIOWithStore(cio, store, bs, multiStore)
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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	pcid, tmpPath, _, err := pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
	require.NoError(t, err)
	tmpFile, err := store.Open(tmpPath)
	require.NoError(t, err)
	defer func() {
		deferErr := tmpFile.Close()
		require.NoError(t, deferErr)
		deferErr = store.Delete(tmpFile.Path())
		require.NoError(t, deferErr)
	}()
	require.NotEqual(t, pcid, cid.Undef)
	bufSize := int64(16) // small buffer to illustrate the logic
	buf := make([]byte, bufSize)
	var readErr error
	padStart := int64(-1)
	loops := int64(-1)
	read := 0
	skipped, err := tmpFile.Seek(tmpFile.Size()/2, io.SeekStart)
	require.NoError(t, err)
	for readErr == nil {
		loops++
		read, readErr = tmpFile.Read(buf)
		for idx := int64(0); idx < int64(read); idx++ {
			if buf[idx] == 0 {
				if padStart == -1 {
					padStart = skipped + loops*bufSize + idx
				}
			} else {
				padStart = -1
			}
		}
	}
	_, err = tmpFile.Seek(0, io.SeekStart)
	require.NoError(t, err)

	var reader io.Reader
	if padStart != -1 {
		reader = io.LimitReader(tmpFile, padStart)
	} else {
		reader = tmpFile
	}

	id, err := pio.ReadPiece(&storeID, reader)
	require.NoError(t, err)
	require.Equal(t, nd3.Cid(), id)
}

func Test_StoreRestoreMemoryBuffer(t *testing.T) {
	tempDir := filestore.OsPath("./tempDir")
	cio := cario.NewCarIO()

	store, err := filestore.NewLocalFileStore(tempDir)
	require.NoError(t, err)

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	pio := pieceio.NewPieceIOWithStore(cio, store, bs, multiStore)

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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	commitment, tmpPath, paddedSize, err := pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
	require.NoError(t, err)
	tmpFile, err := store.Open(tmpPath)
	require.NoError(t, err)
	defer func() {
		deferErr := tmpFile.Close()
		require.NoError(t, deferErr)
		deferErr = store.Delete(tmpFile.Path())
		require.NoError(t, deferErr)
	}()

	_, err = tmpFile.Seek(0, io.SeekStart)
	require.NoError(t, err)

	require.NotEqual(t, commitment, cid.Undef)
	buf := make([]byte, paddedSize)
	_, err = tmpFile.Read(buf)
	require.NoError(t, err)
	buffer := bytes.NewBuffer(buf)
	secondCommitment, err := pieceio.GeneratePieceCIDFromFile(abi.RegisteredSealProof_StackedDrg2KiBV1, buffer, paddedSize)
	require.NoError(t, err)
	require.Equal(t, commitment, secondCommitment)
}

func Test_PieceCommitmentEquivalenceMemoryFile(t *testing.T) {
	tempDir := filestore.OsPath("./tempDir")
	cio := cario.NewCarIO()

	store, err := filestore.NewLocalFileStore(tempDir)
	require.NoError(t, err)

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	multiStore, err := multistore.NewMultiDstore(ds)
	require.NoError(t, err)
	bs := blockstore.NewBlockstore(ds)

	pio := pieceio.NewPieceIOWithStore(cio, store, bs, multiStore)

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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	fcommitment, tmpPath, fpaddedSize, ferr := pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
	defer func() {
		deferErr := store.Delete(tmpPath)
		require.NoError(t, deferErr)
	}()

	mcommitment, mpaddedSize, merr := pio.GeneratePieceCommitment(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
	require.Equal(t, fcommitment, mcommitment)
	require.Equal(t, fpaddedSize, mpaddedSize)
	require.Equal(t, ferr, merr)
	require.NoError(t, ferr)
	require.NoError(t, merr)
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

	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("Links",
			ssb.ExploreIndex(1, ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge()))))
	}).Node()

	t.Run("create temp file fails", func(t *testing.T) {
		fsmock := fsmocks.FileStore{}
		fsmock.On("CreateTemp").Return(nil, fmt.Errorf("Failed"))
		pio := pieceio.NewPieceIOWithStore(nil, &fsmock, bs, multiStore)
		_, _, _, err := pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
		require.Error(t, err)
	})
	t.Run("write CAR fails", func(t *testing.T) {
		tempDir := filestore.OsPath("./tempDir")
		store, err := filestore.NewLocalFileStore(tempDir)
		require.NoError(t, err)

		ciomock := pmocks.CarIO{}
		any := mock.Anything
		ciomock.On("WriteCar", any, any, any, any, any).Return(fmt.Errorf("failed to write car"))
		pio := pieceio.NewPieceIOWithStore(&ciomock, store, bs, multiStore)
		_, _, _, err = pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
		require.Error(t, err)
	})
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
	t.Run("seek fails", func(t *testing.T) {
		cio := cario.NewCarIO()

		fsmock := fsmocks.FileStore{}
		mockfile := fsmocks.File{}

		fsmock.On("CreateTemp").Return(&mockfile, nil).Once()
		fsmock.On("Delete", mock.Anything).Return(nil).Once()

		counter := 0
		size := 0
		mockfile.On("Write", mock.Anything).Run(func(args mock.Arguments) {
			arg := args[0]
			buf := arg.([]byte)
			size := len(buf)
			counter += size
		}).Return(size, nil).Times(17)
		mockfile.On("Size").Return(int64(484))
		mockfile.On("Write", mock.Anything).Return(24, nil).Once()
		mockfile.On("Close").Return(nil).Once()
		mockfile.On("Path").Return(filestore.Path("mock")).Once()
		mockfile.On("Seek", mock.Anything, mock.Anything).Return(int64(0), fmt.Errorf("seek failed"))

		pio := pieceio.NewPieceIOWithStore(cio, &fsmock, bs, multiStore)
		_, _, _, err := pio.GeneratePieceCommitmentToFile(abi.RegisteredSealProof_StackedDrg2KiBV1, nd3.Cid(), node, &storeID)
		require.Error(t, err)
	})
}
