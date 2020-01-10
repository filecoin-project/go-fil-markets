package pieceio_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/filecoin-project/go-fil-markets/filestore"
	fsmocks "github.com/filecoin-project/go-fil-markets/filestore/mocks"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	pmocks "github.com/filecoin-project/go-fil-markets/pieceio/mocks"
	"github.com/filecoin-project/go-fil-markets/pieceio/padreader"
	"github.com/filecoin-project/go-fil-markets/pieceio/sectorcalculator"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"testing"
)

func Test_ThereAndBackAgain(t *testing.T) {
	tempDir := filestore.Path("./tempDir")
	sc := sectorcalculator.NewSectorCalculator(tempDir)
	pr := padreader.NewPadReader()
	cio := cario.NewCarIO()

	store, err := filestore.NewLocalFileStore(tempDir)
	require.NoError(t, err)
	pio := pieceio.NewPieceIO(pr, cio, sc, store)
	require.NoError(t, err)

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

	bytes, filename, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
	require.NoError(t, err)
	for _, b := range bytes {
		require.NotEqual(t, 0, b)
	}
	f, err := os.Open(string(filename))
	require.NoError(t, err)
	info, err := os.Stat(string(filename))
	require.NoError(t, err)
	bufSize := int64(16) // small buffer to illustrate the logic
	buf := make([]byte, bufSize)
	var readErr error
	padStart := int64(-1)
	loops := int64(-1)
	read := 0
	skipped, err := f.Seek(info.Size()/2, io.SeekStart)
	require.NoError(t, err)
	for readErr == nil {
		loops++
		read, readErr = f.Read(buf)
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
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	var reader io.Reader
	if padStart != -1 {
		reader = io.LimitReader(f, padStart)
	} else {
		reader = f
	}

	id, err := pio.ReadPiece(reader, sourceBs)
	os.Remove(string(filename))
	require.NoError(t, err)
	require.Equal(t, nd3.Cid(), id)
}

func Test_StoreRestoreMemoryBuffer(t *testing.T) {
	tempDir := filestore.Path("./tempDir")
	sc := sectorcalculator.NewSectorCalculator(tempDir)
	pr := padreader.NewPadReader()
	cio := cario.NewCarIO()

	store, err := filestore.NewLocalFileStore(tempDir)
	require.NoError(t, err)
	pio := pieceio.NewPieceIO(pr, cio, sc, store)

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

	commitment, filename, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
	require.NoError(t, err)
	for _, b := range commitment {
		require.NotEqual(t, 0, b)
	}
	f, err := os.Open(string(filename))
	require.NoError(t, err)
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	info, err := os.Stat(string(filename))
	require.NoError(t, err)
	buf := make([]byte, info.Size())
	_, err = f.Read(buf)
	require.NoError(t, err)
	buffer := bytes.NewBuffer(buf)
	secondCommitment, err := sc.GeneratePieceCommitment(buffer, uint64(info.Size()))
	require.NoError(t, err)
	require.Equal(t, commitment, secondCommitment)
}

func Test_Failures(t *testing.T) {
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

	t.Run("create temp file fails", func(t *testing.T) {
		fsmock := fsmocks.FileStore{}
		fsmock.On("CreateTemp").Return(nil, fmt.Errorf("Failed"))
		pio := pieceio.NewPieceIO(nil, nil, nil, &fsmock)
		_, _, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
	t.Run("write CAR fails", func(t *testing.T) {
		tempDir := filestore.Path("./tempDir")
		sc := sectorcalculator.NewSectorCalculator(tempDir)
		pr := padreader.NewPadReader()
		store, err := filestore.NewLocalFileStore(tempDir)
		require.NoError(t, err)

		ciomock := pmocks.CarIO{}
		any := mock.Anything
		ciomock.On("WriteCar", any, any, any, any, any).Return(fmt.Errorf("failed to write car"))
		pio := pieceio.NewPieceIO(pr, &ciomock, sc, store)
		_, _, err = pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
	t.Run("padding fails", func(t *testing.T) {
		tempDir := filestore.Path("./tempDir")
		sc := sectorcalculator.NewSectorCalculator(tempDir)
		pr := padreader.NewPadReader()
		cio := cario.NewCarIO()

		fsmock := fsmocks.FileStore{}
		mockfile := fsmocks.File{}

		fsmock.On("CreateTemp").Return(&mockfile, nil).Once()
		fsmock.On("Delete", mock.Anything).Return(nil).Once()

		counter := 0
		size := 0
		mockfile.On("Write", mock.Anything).Run(func (args mock.Arguments) {
			arg := args[0]
			buf := arg.([]byte)
			size := len(buf)
			counter += size
		}).Return(size, nil).Times(17)
		mockfile.On("Size").Return(int64(484))
		mockfile.On("Write", mock.Anything).Return(0, fmt.Errorf("write failed")).Once()
		mockfile.On("Close").Return(nil).Once()
		mockfile.On("Path").Return(filestore.Path("mock")).Once()

		pio := pieceio.NewPieceIO(pr, cio, sc, &fsmock)
		_, _, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
	t.Run("incorrect padding", func(t *testing.T) {
		tempDir := filestore.Path("./tempDir")
		sc := sectorcalculator.NewSectorCalculator(tempDir)
		pr := padreader.NewPadReader()
		cio := cario.NewCarIO()

		fsmock := fsmocks.FileStore{}
		mockfile := fsmocks.File{}

		fsmock.On("CreateTemp").Return(&mockfile, nil).Once()
		fsmock.On("Delete", mock.Anything).Return(nil).Once()

		counter := 0
		size := 0
		mockfile.On("Write", mock.Anything).Run(func (args mock.Arguments) {
			arg := args[0]
			buf := arg.([]byte)
			size := len(buf)
			counter += size
		}).Return(size, nil).Times(17)
		mockfile.On("Size").Return(int64(484))
		mockfile.On("Write", mock.Anything).Return(16, nil).Once()
		mockfile.On("Close").Return(nil).Once()
		mockfile.On("Path").Return(filestore.Path("mock")).Once()

		pio := pieceio.NewPieceIO(pr, cio, sc, &fsmock)
		_, _, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
	t.Run("seek fails", func(t *testing.T) {
		tempDir := filestore.Path("./tempDir")
		sc := sectorcalculator.NewSectorCalculator(tempDir)
		pr := padreader.NewPadReader()
		cio := cario.NewCarIO()

		fsmock := fsmocks.FileStore{}
		mockfile := fsmocks.File{}

		fsmock.On("CreateTemp").Return(&mockfile, nil).Once()
		fsmock.On("Delete", mock.Anything).Return(nil).Once()

		counter := 0
		size := 0
		mockfile.On("Write", mock.Anything).Run(func (args mock.Arguments) {
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

		pio := pieceio.NewPieceIO(pr, cio, sc, &fsmock)
		_, _, err := pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
	t.Run("generate piece commitment fails", func(t *testing.T) {
		tempDir := filestore.Path("./tempDir")
		sc := pmocks.SectorCalculator{}
		pr := padreader.NewPadReader()
		cio := cario.NewCarIO()

		sc.On("GeneratePieceCommitment", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, fmt.Errorf("commitment failed"))

		store, err := filestore.NewLocalFileStore(tempDir)
		require.NoError(t, err)
		pio := pieceio.NewPieceIO(pr, cio, &sc, store)
		_, _, err = pio.GeneratePieceCommitment(sourceBs, nd3.Cid(), node)
		require.Error(t, err)
	})
}