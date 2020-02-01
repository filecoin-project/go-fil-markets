package pieceio

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"

	"github.com/filecoin-project/go-fil-markets/filestore"
)

type PreparedCar interface {
	Size() uint64
	Dump(w io.Writer) error
}

type CarIO interface {
	// WriteCar writes a given payload to a CAR file and into the passed IO stream
	WriteCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, node ipld.Node, w io.Writer) error

	// PrepareCar prepares a car so that it's total size can be calculated without writing it to a file.
	// It can then be written with PreparedCar.Dump
	PrepareCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, node ipld.Node) (PreparedCar, error)

	// LoadCar loads blocks into the a store from a given CAR file
	LoadCar(bs WriteStore, r io.Reader) (cid.Cid, error)
}

type pieceIO struct {
	carIO CarIO
	bs    blockstore.Blockstore
}

func NewPieceIO(carIO CarIO, bs blockstore.Blockstore) PieceIO {
	return &pieceIO{carIO, bs}
}

type pieceIOWithStore struct {
	pieceIO
	store filestore.FileStore
}

func NewPieceIOWithStore(carIO CarIO, store filestore.FileStore, bs blockstore.Blockstore) PieceIOWithStore {
	return &pieceIOWithStore{pieceIO{carIO, bs}, store}
}

func (pio *pieceIO) GeneratePieceCommitment(payloadCid cid.Cid, selector ipld.Node) ([]byte, uint64, error) {
	preparedCar, err := pio.carIO.PrepareCar(context.Background(), pio.bs, payloadCid, selector)
	if err != nil {
		return nil, 0, err
	}
	pieceSize := uint64(preparedCar.Size())
	r, w, err := os.Pipe()
	if err != nil {
		return nil, 0, err
	}
	var stop sync.WaitGroup
	stop.Add(1)
	var werr error
	go func() {
		defer stop.Done()
		werr = preparedCar.Dump(w)
		err := w.Close()
		if werr == nil && err != nil {
			werr = err
		}
	}()
	commitment, paddedSize, err := GeneratePieceCommitment(r, pieceSize)
	closeErr := r.Close()
	if err != nil {
		return nil, 0, err
	}
	if closeErr != nil {
		return nil, 0, closeErr
	}
	stop.Wait()
	if werr != nil {
		return nil, 0, werr
	}
	return commitment, paddedSize, nil
}

func (pio *pieceIOWithStore) GeneratePieceCommitmentToFile(payloadCid cid.Cid, selector ipld.Node) ([]byte, filestore.Path, uint64, error) {
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
