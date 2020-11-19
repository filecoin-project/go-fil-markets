package pieceio

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"
	"github.com/prometheus/common/log"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
)

type PreparedCar interface {
	Size() uint64
	Dump(w io.Writer) error
}

type CarIO interface {
	// WriteCar writes a given payload to a CAR file and into the passed IO stream
	WriteCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, node ipld.Node, w io.Writer, userOnNewCarBlocks ...car.OnNewCarBlockFunc) error

	// PrepareCar prepares a car so that its total size can be calculated without writing it to a file.
	// It can then be written with PreparedCar.Dump
	PrepareCar(ctx context.Context, bs ReadStore, payloadCid cid.Cid, node ipld.Node, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (PreparedCar, error)

	// LoadCar loads blocks into the a store from a given CAR file
	LoadCar(bs WriteStore, r io.Reader) (cid.Cid, error)
}

type pieceIO struct {
	carIO      CarIO
	bs         blockstore.Blockstore
	multiStore MultiStore
}

type MultiStore interface {
	Get(i multistore.StoreID) (*multistore.Store, error)
}

func NewPieceIO(carIO CarIO, bs blockstore.Blockstore, multiStore MultiStore) PieceIO {
	return &pieceIO{carIO, bs, multiStore}
}

func (pio *pieceIO) GeneratePieceReader(payloadCid cid.Cid, selector ipld.Node, storeID *multistore.StoreID, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (io.ReadCloser, uint64, error, <-chan error) {
	bstore, err := pio.bstore(storeID)
	if err != nil {
		return nil, 0, err, nil
	}
	preparedCar, err := pio.carIO.PrepareCar(context.Background(), bstore, payloadCid, selector, userOnNewCarBlocks...)
	if err != nil {
		return nil, 0, err, nil
	}
	pieceSize := uint64(preparedCar.Size())
	r, w, err := os.Pipe()
	if err != nil {
		return nil, 0, err, nil
	}
	writeErr := make(chan error, 1)
	go func() {
		werr := preparedCar.Dump(w)
		err := w.Close()
		if werr == nil && err != nil {
			werr = err
		}
		writeErr <- werr
	}()
	return r, pieceSize, nil, writeErr
}

func (pio *pieceIO) GeneratePieceCommitment(rt abi.RegisteredSealProof, payloadCid cid.Cid, selector ipld.Node, storeID *multistore.StoreID, userOnNewCarBlocks ...car.OnNewCarBlockFunc) (cid.Cid, abi.UnpaddedPieceSize, error) {
	r, pieceSize, err, writeErrChan := pio.GeneratePieceReader(payloadCid, selector, storeID, userOnNewCarBlocks...)
	if err != nil {
		return cid.Undef, 0, err
	}
	commitment, paddedSize, err := GeneratePieceCommitment(rt, r, pieceSize)
	closeErr := r.Close()
	if err != nil {
		return cid.Undef, 0, err
	}
	if closeErr != nil {
		return cid.Undef, 0, closeErr
	}
	werr := <-writeErrChan
	if werr != nil {
		return cid.Undef, 0, werr
	}
	return commitment, paddedSize, nil
}

func GeneratePieceCommitment(rt abi.RegisteredSealProof, rd io.Reader, pieceSize uint64) (cid.Cid, abi.UnpaddedPieceSize, error) {
	paddedReader, paddedSize := padreader.New(rd, pieceSize)
	commitment, err := GeneratePieceCIDFromFile(rt, paddedReader, paddedSize)
	if err != nil {
		return cid.Undef, 0, err
	}
	return commitment, paddedSize, nil
}

func (pio *pieceIO) ReadPiece(storeID *multistore.StoreID, r io.Reader) (cid.Cid, error) {
	bstore, err := pio.bstore(storeID)
	if err != nil {
		return cid.Undef, err
	}
	return pio.carIO.LoadCar(bstore, r)
}

func (pio *pieceIO) bstore(storeID *multistore.StoreID) (blockstore.Blockstore, error) {
	if storeID == nil {
		return pio.bs, nil
	}
	store, err := pio.multiStore.Get(*storeID)
	if err != nil {
		return nil, err
	}
	return store.Bstore, nil
}

func ToReadableFile(r io.Reader, n int64) (*os.File, func() error, error) {
	f, ok := r.(*os.File)
	if ok {
		return f, func() error { return nil }, nil
	}

	var w *os.File

	f, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	var wait sync.Mutex
	var werr error

	wait.Lock()
	go func() {
		defer wait.Unlock()

		var copied int64
		copied, werr = io.CopyN(w, r, n)
		if werr != nil {
			log.Warnf("toReadableFile: copy error: %+v", werr)
		}

		err := w.Close()
		if werr == nil && err != nil {
			werr = err
			log.Warnf("toReadableFile: close error: %+v", err)
			return
		}
		if copied != n {
			log.Warnf("copied different amount than expected: %d != %d", copied, n)
			werr = xerrors.Errorf("copied different amount than expected: %d != %d", copied, n)
		}
	}()

	return f, func() error {
		wait.Lock()
		return werr
	}, nil
}

func GeneratePieceCIDFromFile(proofType abi.RegisteredSealProof, piece io.Reader, pieceSize abi.UnpaddedPieceSize) (cid.Cid, error) {
	f, werr, err := ToReadableFile(piece, int64(pieceSize))
	if err != nil {
		return cid.Undef, err
	}

	pieceCID, err := ffi.GeneratePieceCIDFromFile(proofType, f, pieceSize)
	if err != nil {
		return cid.Undef, err
	}

	return pieceCID, werr()
}
