package padreader

import (
    ffi "github.com/filecoin-project/filecoin-ffi"
    "github.com/filecoin-project/go-fil-components/pieceio"
    "io"
    "math/bits"
)

type padReader struct {
}

func NewPadReader() pieceio.PadReader {
    return &padReader{}
}

// Functions bellow copied from lotus/lib/padreader/padreader.go
func (p padReader) PaddedSize(size uint64) uint64 {
    logv := 64 - bits.LeadingZeros64(size)

    sectSize := uint64(1 << logv)
    bound := ffi.GetMaxUserBytesPerStagedSector(sectSize)
    if size <= bound {
        return bound
    }

    return ffi.GetMaxUserBytesPerStagedSector(1 << (logv + 1))
}

func (p padReader) NewPaddedReader(r io.Reader, size uint64) (io.Reader, uint64) {
    padSize := p.PaddedSize(size)
    reader := io.MultiReader(
        io.LimitReader(r, int64(size)),
        io.LimitReader(r, int64(padSize - size)),
    )
    return reader, padSize
}
