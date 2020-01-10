package sectorcalculator

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"io"
	"io/ioutil"
	"os"
)

type sectorCalculator struct {
	tempDir filestore.Path
}

func NewSectorCalculator(tempDir filestore.Path) pieceio.SectorCalculator {
	return &sectorCalculator{tempDir}
}

func (s sectorCalculator) GeneratePieceCommitment(piece io.Reader, pieceSize uint64) ([]byte, error) {
	f, ok := piece.(*os.File) // try to avoid yet another temp file
	var err error
	if !ok {
		f, err = ioutil.TempFile(string(s.tempDir), "")
		if err != nil {
			return nil, err
		}
		defer func() { // cleanup
			f.Close()
			os.Remove(f.Name())
		}()
		_, err = io.Copy(f, io.LimitReader(piece, int64(pieceSize)))
		if err != nil {
			return nil, err
		}
		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
	} else {
		defer f.Close()
	}
	commP, err := ffi.GeneratePieceCommitmentFromFile(f, pieceSize)
	return commP[:], err
}
