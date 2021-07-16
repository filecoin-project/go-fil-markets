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
	io.Closer
}

// NewReadOnlyFileStore returns a Filestore backed by the given CAR file that clients can read Unixfs DAG blocks from.
// The intermediate blocks of the Unixfs DAG are returned as is from the given CAR file.
// For the leaf nodes of the Unixfs DAG, if  `PosInfo`(filepath, offset, size) nodes have been written to the CAR file,
// the filestore will read the `PosInfo` node from the CAR file and then resolve the actual raw leaf data from the file
// referenced by the `PosInfo` node.
//
// Note that if the given CAR file does NOT contain any `PosInfo` nodes and contains all Unixfs DAG blocks
// as is, the filestore will return all blocks as is from the given CAR file i.e. in such a case,
// the Filestore will simply act as a pass-through read only CAR blockstore.
func NewReadOnlyFileStore(carFilePath string) (*FileStore, error) {
	// Open a readOnly blockstore that wraps the given CAR file.
	rdOnly, err := blockstore.OpenReadOnly(carFilePath)
	if err != nil {
		return nil, xerrors.Errorf("failed to open read-only blockstore: %w", err)
	}

	// adapt the CAR blockstore to a `key-value datastore` to persist the (cid -> `PosInfo`) infromation
	// for the leaf nodes of a Unixfs DAG i.e. the nodes that correspond to fix sized chunks of the user's raw file.
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

// NewReadWriteFileStore returns a Filestore that clients can write Unixfs DAG blocks to and read  Unixfs DAG blocks from.
// The Filestore will persist "intermediate" Unixfs DAG blocks as is to the given CARv2 file.
// For the leaf nodes of the UnixFS DAG which correspond to fixed size chunks of the user file ,
// the Filestore will store the `PosInfo` Node specifying the (user filePath, offset, size) information for the chunk
// in the CARv2 file.
//
// Note that if the client does NOT write any `PosInfo` nodes to the Filestore, the backing CARv2 file will contain
// all blocks as is i.e. in such a case, the Filestore will simply act as a pass-through read-write CAR Blockstore.
func NewReadWriteFileStore(carV2FilePath string, roots []cid.Cid) (*FileStore, error) {
	rw, err := blockstore.OpenReadWrite(carV2FilePath, roots)
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

type carV2BSCloser struct {
	rw *blockstore.ReadWrite
}

func (c *carV2BSCloser) Close() error {
	if err := c.rw.Finalize(); err != nil {
		return xerrors.Errorf("failed to finalize read-write blockstore: %w", err)
	}

	return nil
}
