package filestorecaradapter

import (
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/filestorecaradapter/internal"
)

type FileStore struct {
	bstore.Blockstore
	closer io.Closer
}

func NewReadOnlyFileStore(carFilePath string) (*FileStore, error) {
	rdOnly, err := blockstore.OpenReadOnly(carFilePath)
	if err != nil {
		return nil, xerrors.Errorf("failed to open read-only blockstore: %w", err)
	}

	adapter := internal.BlockstoreToDSBatchingAdapter(rdOnly)
	fm := filestore.NewFileManager(adapter, "/")
	fm.AllowFiles = true
	fstore := filestore.NewFilestore(rdOnly, fm)
	bs := bstore.NewIdStore(fstore)

	return &FileStore{
		bs,
		rdOnly,
	}, nil
}

func NewReadWriteFileStore(carV2FilePath string, roots []cid.Cid) (*FileStore, error) {
	rw, err := blockstore.NewReadWrite(carV2FilePath, roots)
	if err != nil {
		return nil, xerrors.Errorf("failed to open read-write blockstore: %w", err)
	}

	adapter := internal.BlockstoreToDSBatchingAdapter(rw)
	fm := filestore.NewFileManager(adapter, "/")
	fm.AllowFiles = true
	fstore := filestore.NewFilestore(rw, fm)
	bs := bstore.NewIdStore(fstore)

	return &FileStore{
		bs,
		&carV2BSCloser{rw},
	}, nil
}

func (fs *FileStore) Close() error {
	return fs.closer.Close()
}

type carV2BSCloser struct {
	rw *blockstore.ReadWrite
}

func (c *carV2BSCloser) Close() error {
	if err := c.rw.Finalize(); err != nil {
		return xerrors.Errorf("failed to finalize read-write blockstore: %w", err)
	}

	return nil
}
