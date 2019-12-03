package pieceio_test

import (
	"context"
	"io"
	"testing"

	"github.com/filecoin-project/go-fil-components/pieceio"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
)

type mockSectorCalculator struct{}

// GeneratePieceCommitment takes a PADDED io stream and a total size and generates a commP
func (sc *mockSectorCalculator) GeneratePieceCommitment(piece io.Reader, pieceSize uint64) ([]byte, error) {
	panic("not implemented")
}

type mockCarIO struct{}

// WriteCar writes a given payload to a CAR file and into the passed IO stream
func (cio *mockCarIO) WriteCar(ctx context.Context, bs pieceio.ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) error {
	panic("not implemented")
}

// LoadCar loads blocks into the a store from a given CAR file
func (cio *mockCarIO) LoadCar(bs pieceio.WriteStore, r io.Reader) (cid.Cid, error) {
	panic("not implemented")
}

type mockPadReader struct{}

// PaddedSize returns the expected size of a piece after it's been padded
func (pr *mockPadReader) PaddedSize(size uint64) uint64 {
	panic("not implemented")
}

// NewPaddedReader takes an io.Reader and an unpadded size and returns a reader
// with padded zeros
func (pr *mockPadReader) NewPaddedReader(r io.Reader, size uint64) (io.Reader, uint64) {
	panic("not implemented")
}

func TestGeneratePieceCommitment(t *testing.T) {
	sc := &mockSectorCalculator{}
	cio := &mockCarIO{}
	pr := &mockPadReader{}

	_ = pieceio.NewPieceIO(pr, cio, sc)

	// Write test to demonstrate generate piece commitment generates a car to a temporary write buffer,
	// then pads it, then generates a piece commitment
}

func TestWritePayload(t *testing.T) {
	sc := &mockSectorCalculator{}
	cio := &mockCarIO{}
	pr := &mockPadReader{}

	_ = pieceio.NewPieceIO(pr, cio, sc)

	// Write test to demonstrate write payload generates a car,
	// then pads it, writes to the writer, then generates a piece commitment
}

func TestReadPiece(t *testing.T) {
	sc := &mockSectorCalculator{}
	cio := &mockCarIO{}
	pr := &mockPadReader{}

	_ = pieceio.NewPieceIO(pr, cio, sc)

	// Write test to demonstrate read piece reads in a piece, unpads it,
	// and loads to the blockstore from car file
}
