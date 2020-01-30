package pieceio

import (
	"context"
	"io"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"

	"github.com/filecoin-project/go-fil-markets/filestore"
)

type CarIO interface {
	// WriteCar writes a given payload to a CAR file and into the passed IO stream
	WriteCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, selector ipld.Node, w io.Writer) error
	// LoadCar loads blocks into the a store from a given CAR file
	LoadCar(bs WriteStore, r io.Reader) (cid.Cid, error)
}

type pieceIO struct {
	carIO CarIO
	store filestore.FileStore
	bs    blockstore.Blockstore
}

func NewPieceIO(carIO CarIO, store filestore.FileStore, bs blockstore.Blockstore) PieceIO {
	return &pieceIO{carIO, store, bs}
}

func (pio *pieceIO) GeneratePieceCommitment(payloadCid cid.Cid, selector ipld.Node) ([]byte, filestore.Path, uint64, error) {
	f, err := pio.store.CreateTemp()
	if err != nil {
		return nil, "", 0, err
	}
	cleanup := func() {
		f.Close()
		_ = pio.store.Delete(f.Path())
	}
	err = pio.carIO.WriteCar(context.Background(), pio.bs, payloadCid, selector, f)
	if err != nil {
		cleanup()
		return nil, "", 0, err
	}
	pieceSize := uint64(f.Size())
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		cleanup()
		return nil, "", 0, err
	}
	commitment, paddedSize, err := GeneratePieceCommitment(f, pieceSize)
	if err != nil {
		cleanup()
		return nil, "", 0, err
	}
	_ = f.Close()
	return commitment, f.Path(), paddedSize, nil
}

func GeneratePieceCommitment(rd io.Reader, pieceSize uint64) ([]byte, uint64, error) {
	paddedReader, paddedSize := padreader.New(rd, pieceSize)
	commitment, err := sectorbuilder.GeneratePieceCommitment(paddedReader, paddedSize)
	if err != nil {
		return nil, 0, err
	}
	return commitment[:], paddedSize, nil
}

func (pio *pieceIO) ReadPiece(r io.Reader) (cid.Cid, error) {
	return pio.carIO.LoadCar(pio.bs, r)
}
