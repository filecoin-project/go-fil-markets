package pieceio_test

import (
	"bytes"
	"context"
	"github.com/filecoin-project/go-fil-components/filestore"
	"github.com/filecoin-project/go-fil-components/pieceio"
	"github.com/filecoin-project/go-fil-components/pieceio/cario"
	"github.com/filecoin-project/go-fil-components/pieceio/padreader"
	"github.com/filecoin-project/go-fil-components/pieceio/sectorcalculator"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
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

	pio := pieceio.NewPieceIO(pr, cio, sc, tempDir)

	sourceBserv := dstest.Bserv()
	sourceBs := sourceBserv.Blockstore()
	dserv := dag.NewDAGService(sourceBserv)
	a := dag.NewRawNode([]byte("aaaa"))
	b := dag.NewRawNode([]byte("bbbb"))
	c := dag.NewRawNode([]byte("cccc"))

	nd1 := &dag.ProtoNode{}
	nd1.AddNodeLink("cat", a)

	nd2 := &dag.ProtoNode{}
	nd2.AddNodeLink("first", nd1)
	nd2.AddNodeLink("dog", b)

	nd3 := &dag.ProtoNode{}
	nd3.AddNodeLink("second", nd2)
	nd3.AddNodeLink("bear", c)

	ctx := context.Background()
	dserv.Add(ctx, a)
	dserv.Add(ctx, b)
	dserv.Add(ctx, c)
	dserv.Add(ctx, nd1)
	dserv.Add(ctx, nd2)
	dserv.Add(ctx, nd3)

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
					padStart = skipped + loops * bufSize + idx
				}
			} else {
				padStart = -1
			}
		}
	}
	f.Seek(0, io.SeekStart)
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

	pio := pieceio.NewPieceIO(pr, cio, sc, tempDir)

	sourceBserv := dstest.Bserv()
	sourceBs := sourceBserv.Blockstore()
	dserv := dag.NewDAGService(sourceBserv)
	a := dag.NewRawNode([]byte("aaaa"))
	b := dag.NewRawNode([]byte("bbbb"))
	c := dag.NewRawNode([]byte("cccc"))

	nd1 := &dag.ProtoNode{}
	nd1.AddNodeLink("cat", a)

	nd2 := &dag.ProtoNode{}
	nd2.AddNodeLink("first", nd1)
	nd2.AddNodeLink("dog", b)

	nd3 := &dag.ProtoNode{}
	nd3.AddNodeLink("second", nd2)
	nd3.AddNodeLink("bear", c)

	ctx := context.Background()
	dserv.Add(ctx, a)
	dserv.Add(ctx, b)
	dserv.Add(ctx, c)
	dserv.Add(ctx, nd1)
	dserv.Add(ctx, nd2)
	dserv.Add(ctx, nd3)

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
	defer func () {
		f.Close()
		os.Remove(f.Name())
	}()
	info, err := os.Stat(string(filename))
	buf := make([]byte, info.Size())
	f.Read(buf)
	buffer := bytes.NewBuffer(buf)
	secondCommitment, err := sc.GeneratePieceCommitment(buffer, uint64(info.Size()))
	require.NoError(t, err)
	require.Equal(t, commitment, secondCommitment)
}