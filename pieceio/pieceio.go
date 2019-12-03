package pieceio

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
)

type SectorCalculator interface {
	// GeneratePieceCommitment takes a PADDED io stream and a total size and generates a commP
	GeneratePieceCommitment(piece io.Reader, pieceSize uint64) ([]byte, error)
}

type PadReader interface {
	// PaddedSize returns the expected size of a piece after it's been padded
	PaddedSize(size uint64) uint64
	// NewPaddedReader takes an io.Reader and an unpadded size and returns a reader
	// with padded zeros
	NewPaddedReader(r io.Reader, size uint64) (io.Reader, uint64)
}

type CarIO interface {
	// WriteCar writes a given payload to a CAR file and into the passed IO stream
	WriteCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) error
	// LoadCar loads blocks into the a store from a given CAR file
	LoadCar(bs WriteStore, r io.Reader) (cid.Cid, error)
}

type pieceIO struct {
	padReader        PadReader
	carIO            CarIO
	sectorCalculator SectorCalculator
}

func NewPieceIO(padReader PadReader, carIO CarIO, sectorCalculator SectorCalculator) PieceIO {
	return &pieceIO{padReader, carIO, sectorCalculator}
}

func (pio *pieceIO) GeneratePieceCommitment(bs ReadStore, payloadCid cid.Cid, selector ipld.Node) ([]byte, error) {
	return nil, nil
}

func (pio *pieceIO) WritePayload(bs ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) ([]byte, error) {
	return nil, nil
}
func (pio *pieceIO) ReadPiece(r io.Reader, bs WriteStore) (cid.Cid, error) {
	return cid.Cid{}, nil
}
