package padreader

import (
	"io"
	"math/bits"

	ffi "github.com/filecoin-project/filecoin-ffi"
)

// Functions bellow copied from lotus/lib/padreader/padreader.go
func PaddedSize(size uint64) uint64 {
	logv := 64 - bits.LeadingZeros64(size)

	sectSize := uint64(1 << logv)
	bound := ffi.GetMaxUserBytesPerStagedSector(sectSize)
	if size <= bound {
		return bound
	}

	return ffi.GetMaxUserBytesPerStagedSector(1 << (logv + 1))
}

type nullReader struct{}

func (nr nullReader) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = 0
	}
	return len(b), nil
}

func NewPaddedReader(r io.Reader, size uint64) (io.Reader, uint64) {
	padSize := PaddedSize(size)

	return io.MultiReader(
		io.LimitReader(r, int64(size)),
		io.LimitReader(nullReader{}, int64(padSize-size)),
	), padSize
}
