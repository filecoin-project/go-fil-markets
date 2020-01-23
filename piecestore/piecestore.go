package piecestore

import (
	"bytes"
	"fmt"

	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

var DSPiecePrefix = "/storagemarket/pieces"
var DSCIDPrefix = "/storagemarket/cid-infos"

func NewPieceStore(ds datastore.Batching) PieceStore {
	return &pieceStore{
		pieces:   statestore.New(namespace.Wrap(ds, datastore.NewKey(DSPiecePrefix))),
		cidInfos: statestore.New(namespace.Wrap(ds, datastore.NewKey(DSCIDPrefix))),
	}
}

type pieceStore struct {
	pieces   *statestore.StateStore
	cidInfos *statestore.StateStore
}

func (ps *pieceStore) AddDealForPiece(pieceCID []byte, dealInfo DealInfo) error {
	// Do we need to de-dupe or anything here?
	return ps.mutatePieceInfo(pieceCID, func(pi *PieceInfo) error {
		for _, di := range pi.Deals {
			if di == dealInfo {
				return nil
			}
		}
		pi.Deals = append(pi.Deals, dealInfo)
		return nil
	})
}

func (ps *pieceStore) AddPieceBlockLocations(pieceCID []byte, blockLocations map[cid.Cid]BlockLocation) error {
	for c, blockLocation := range blockLocations {
		err := ps.mutateCIDInfo(c, func(ci *CIDInfo) error {
			for _, pbl := range ci.PieceBlockLocations {
				if bytes.Equal(pbl.PieceCID, pieceCID) && pbl.BlockLocation == blockLocation {
					return nil
				}
			}
			ci.PieceBlockLocations = append(ci.PieceBlockLocations, PieceBlockLocation{blockLocation, pieceCID})
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (ps *pieceStore) GetPieceInfo(pieceCID []byte) (PieceInfo, error) {
	var out PieceInfo
	if err := ps.pieces.Get(newKey(pieceCID)).Get(&out); err != nil {
		return PieceInfo{}, err
	}
	return out, nil
}

func (ps *pieceStore) GetCIDInfo(payloadCID cid.Cid) (CIDInfo, error) {
	var out CIDInfo
	if err := ps.cidInfos.Get(payloadCID).Get(&out); err != nil {
		return CIDInfo{}, err
	}
	return out, nil
}

func (ps *pieceStore) ensurePieceInfo(pieceCID []byte) error {
	has, err := ps.pieces.Has(newKey(pieceCID))

	if err != nil {
		return err
	}
	if has {
		return nil
	}

	pieceInfo := PieceInfo{PieceCID: pieceCID}
	return ps.pieces.Begin(newKey(pieceCID), &pieceInfo)
}

func (ps *pieceStore) ensureCIDInfo(c cid.Cid) error {
	has, err := ps.cidInfos.Has(c)

	if err != nil {
		return err
	}

	if has {
		return nil
	}

	cidInfo := CIDInfo{CID: c}
	return ps.cidInfos.Begin(c, &cidInfo)
}

func (ps *pieceStore) mutatePieceInfo(pieceCID []byte, mutator interface{}) error {
	err := ps.ensurePieceInfo(pieceCID)
	if err != nil {
		return err
	}

	return ps.pieces.Get(newKey(pieceCID)).Mutate(mutator)
}

func (ps *pieceStore) mutateCIDInfo(c cid.Cid, mutator interface{}) error {
	err := ps.ensureCIDInfo(c)
	if err != nil {
		return err
	}

	return ps.cidInfos.Get(c).Mutate(mutator)
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
