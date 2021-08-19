package providerstates

import (
	"strings"
	"testing"

	"io/ioutil"

	"github.com/stretchr/testify/require"
)

func TestSafeReaderCloser(t *testing.T) {
	r := strings.NewReader("test")

	sr := &safeReaderCloser{
		r: r,
		c: ioutil.NopCloser(r),
	}

	// valid read
	bz := make([]byte, 3)
	n, err := sr.Read(bz)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "tes", string(bz))

	// read after close
	require.NoError(t, sr.Close())
	_, err = sr.Read(make([]byte, 1))
	require.Contains(t, err.Error(), "read from closed reader")
}
