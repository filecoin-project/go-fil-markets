package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(SignedStorageAsk{})
	cbor.RegisterCborType(StorageAsk{})
}

//go:generate cbor-gen-for SignedStorageAsk StorageAsk

type SignedStorageAsk struct {
	Ask       *StorageAsk
	Signature *Signature
}

type StorageAsk struct {
	// Price per GiB / Epoch
	Price tokenamount.TokenAmount

	MinPieceSize uint64
	Miner        address.Address
	Timestamp    uint64
	Expiry       uint64
	SeqNo        uint64
}
