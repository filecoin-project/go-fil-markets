package shared_testutil

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

type TestRetrievalBlockstoreAccessor struct {
	Blockstore bstore.Blockstore
}

func (t *TestRetrievalBlockstoreAccessor) Get(rootCid cid.Cid) (bstore.Blockstore, error) {
	return t.Blockstore, nil
}

func (t *TestRetrievalBlockstoreAccessor) Close(rootCid cid.Cid) error {
	return nil
}

func NewTestRetrievalBlockstoreAccessor() *TestRetrievalBlockstoreAccessor {
	return &TestRetrievalBlockstoreAccessor{
		Blockstore: bstore.NewBlockstore(datastore.NewMapDatastore()),
	}
}
