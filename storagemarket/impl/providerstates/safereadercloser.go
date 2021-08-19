package providerstates

import (
	"io"
	"sync"

	"golang.org/x/xerrors"
)

type safeReaderCloser struct {
	r io.Reader
	c io.Closer

	lk       sync.Mutex
	isClosed bool
}

func (s *safeReaderCloser) Read(p []byte) (int, error) {
	s.lk.Lock()

	if s.isClosed {
		s.lk.Unlock()
		return 0, xerrors.New("read from closed reader")
	}
	s.lk.Unlock()

	return s.r.Read(p)
}

func (s *safeReaderCloser) Close() error {
	s.lk.Lock()
	defer s.lk.Unlock()

	if !s.isClosed {
		s.isClosed = true
		return s.c.Close()
	}

	return nil
}
