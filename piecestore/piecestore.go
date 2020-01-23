package piecestore

import (
	"fmt"

	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

var DSPrefix = "/storagemarket/pieces"

func NewPieceStore(ds datastore.Batching) PieceStore {
	return &pieceStore{
		store: statestore.New(namespace.Wrap(ds, datastore.NewKey(DSPrefix))),
	}
}

type pieceStore struct {
	store *statestore.StateStore
}

func (ps *pieceStore) AddDealForPiece(pieceCID []byte, dealInfo DealInfo) error {
	// Do we need to de-dupe or anything here?
	return ps.mutatePieceInfo(pieceCID, func(pi *PieceInfo) error {
		pi.Deals = append(pi.Deals, dealInfo)
		return nil
	})
}

func (ps *pieceStore) AddBlockInfosToPiece(pieceCID []byte, blockInfos []BlockInfo) error {
	// Do we need to de-dupe or anything here?
	return ps.mutatePieceInfo(pieceCID, func(pi *PieceInfo) error {
		pi.Blocks = blockInfos
		return nil
	})
}

func (ps *pieceStore) HasBlockInfo(pieceCID []byte) (bool, error) {
	pi, err := ps.GetPieceInfo(pieceCID)
	if err != nil {
		return false, err
	}

	return len(pi.Blocks) > 0, err
}

func (ps *pieceStore) HasDealInfo(pieceCID []byte) (bool, error) {
	pi, err := ps.GetPieceInfo(pieceCID)
	if err != nil {
		return false, err
	}

	return len(pi.Deals) > 0, nil
}

func (ps *pieceStore) GetPieceInfo(pieceCID []byte) (PieceInfo, error) {
	var out PieceInfo
	if err := ps.store.Get(newKey(pieceCID)).Get(&out); err != nil {
		return PieceInfo{}, err
	}
	return out, nil
}

func (ps *pieceStore) ensurePieceInfo(pieceCID []byte) (PieceInfo, error) {
	pieceInfo, err := ps.GetPieceInfo(pieceCID)

	if err == nil {
		return pieceInfo, nil
	}

	pieceInfo = PieceInfo{PieceCID: pieceCID}
	err = ps.store.Begin(newKey(pieceCID), &pieceInfo)

	return pieceInfo, err
}

func (ps *pieceStore) mutatePieceInfo(pieceCID []byte, mutator interface{}) error {
	_, err := ps.ensurePieceInfo(pieceCID)
	if err != nil {
		return err
	}

	return ps.store.Get(newKey(pieceCID)).Mutate(mutator)
}

func newKey(pieceCID []byte) fmt.Stringer {
	return &pieceStoreKey{pieceCID}
}

type pieceStoreKey struct {
	cid []byte
}

func (k *pieceStoreKey) String() string {
	return string(k.cid)
}
