package padreader

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-fil-components/pieceio"
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
