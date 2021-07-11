package dagstore

import (
	"bufio"
	"context"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	carv2bs "github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-car/v2/index"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/carstore"
)

type MockDagStoreWrapper struct {
	api LotusMountAPI
}

func NewMockDagStoreWrapper(api LotusMountAPI) *MockDagStoreWrapper {
	return &MockDagStoreWrapper{api: api}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string) error {
	return nil
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	// Fetch the unsealed piece
	r, err := m.api.FetchUnsealedPiece(ctx, pieceCid)
	if err != nil {
		return nil, xerrors.Errorf("fetching unsealed piece with CID %s: %w", pieceCid, err)
	}

	// Write the piece to a file
	tmpFile, err := os.CreateTemp("", "dagstoretmp")
	if err != nil {
		return nil, xerrors.Errorf("creating temp file for piece CID %s: %w", pieceCid, err)
	}

	_, err = io.Copy(tmpFile, r)
	if err != nil {
		return nil, xerrors.Errorf("copying read stream to temp file for piece CID %s: %w", pieceCid, err)
	}

	err = tmpFile.Close()
	if err != nil {
		return nil, xerrors.Errorf("closing temp file for piece CID %s: %w", pieceCid, err)
	}

	// Get a blockstore from the CAR file
	return getBlockstore(tmpFile.Name())
}

func getBlockstore(path string) (carstore.ClosableBlockstore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to read file %s: %w", path, err)
	}

	// Get the file header
	hd, _, err := car.ReadHeader(bufio.NewReader(f))
	if err != nil {
		return nil, xerrors.Errorf("failed to read CAR header: %w", err)
	}

	// we read the file above to read the header -> seek to the start to be able to read he file again.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, xerrors.Errorf("failed to seek: %w", err)
	}

	// Get the CAR file, depending on the version
	switch hd.Version {
	case 1:
		idx, err := index.Generate(f)
		if err != nil {
			return nil, xerrors.Errorf("failed to generate index from %s: %w", path, err)
		}
		// we read the file above to generate the Index -> seek to the start to be able to read he file again.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, xerrors.Errorf("failed to seek: %w", err)
		}
		return carv2bs.NewReadOnly(f, idx)

	case 2:
		return carv2bs.OpenReadOnly(path)
	}

	return nil, xerrors.Errorf("unrecognized version %d", hd.Version)
}

var _ DagStoreWrapper = (*MockDagStoreWrapper)(nil)
