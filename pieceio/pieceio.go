package pieceio

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-fil-components/filestore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"io"
	"io/ioutil"
	"os"
)

type SectorCalculator interface {
	// GeneratePieceCommitment takes a PADDED io stream and a total size and generates a commP
	GeneratePieceCommitment(piece io.Reader, pieceSize uint64) ([]byte, error)
}

type PadReader interface {
	// PaddedSize returns the expected size of a piece after it's been padded
	PaddedSize(size uint64) uint64
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
	tempDir          filestore.Path
}

func NewPieceIO(padReader PadReader, carIO CarIO, sectorCalculator SectorCalculator, tempDir filestore.Path) PieceIO {
	return &pieceIO{padReader, carIO, sectorCalculator, tempDir}
}

func (pio *pieceIO) GeneratePieceCommitment(bs ReadStore, payloadCid cid.Cid, selector ipld.Node) ([]byte, filestore.Path, error) {
	f, err := ioutil.TempFile(string(pio.tempDir), "")
	if err != nil {
		return nil, "", err
	}
	err = pio.carIO.WriteCar(context.Background(), bs, payloadCid, selector, f)
	if err != nil {
		os.Remove(f.Name())
		return nil, "", err
	}
	fi, err := f.Stat()
	if err != nil {
		os.Remove(f.Name())
		return nil, "", err
	}
	pieceSize := uint64(fi.Size())
	paddedSize := pio.padReader.PaddedSize(pieceSize)
	remaining := paddedSize - pieceSize
	padbuf := make([]byte, remaining)
	padded, err := f.Write(padbuf)
	if err != nil {
		os.Remove(f.Name())
		return nil, "", err
	}
	if uint64(padded) != remaining {
		os.Remove(f.Name())
		return nil, "", fmt.Errorf("wrote %d byte of padding while expecting %d to be written", padded, remaining)
	}
	f.Seek(0, io.SeekStart)
	commitment, err := pio.sectorCalculator.GeneratePieceCommitment(f, paddedSize)
	if err != nil {
		os.Remove(f.Name())
		return nil, "", err
	}
	return commitment, filestore.Path(f.Name()), nil
}

func (pio *pieceIO) WritePayload(bs ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) ([]byte, error) {
	return nil, pio.carIO.WriteCar(context.Background(), bs, payloadCid, selector, w)
}
func (pio *pieceIO) ReadPiece(r io.Reader, bs WriteStore) (cid.Cid, error) {
	return pio.carIO.LoadCar(bs, r)
}
