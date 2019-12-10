package sectorcalculator

import (
    ffi "github.com/filecoin-project/filecoin-ffi"
    "github.com/filecoin-project/go-fil-components/filestore"
    "github.com/filecoin-project/go-fil-components/pieceio"
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
    f := piece.(*os.File) // try to avoid yet another temp file
    if f == nil {
        f, err := ioutil.TempFile(string(s.tempDir), "")
        if err != nil {
            return nil, err
        }
        _, err = io.Copy(f, io.LimitReader(piece, int64(pieceSize)))
        if err != nil {
            return nil, err
        }
    }
    commP, err := ffi.GeneratePieceCommitmentFromFile(f, pieceSize)
    f.Close()
    return commP[:], err
}
