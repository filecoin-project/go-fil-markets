package shared_testutil

import (
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/minio/sha256-simd"
	mh "github.com/multiformats/go-multihash"

	"github.com/filecoin-project/go-state-types/abi"
)

// make a cid directly with a given input and prefix
func MakeCID(input string, prefix *cid.Prefix) cid.Cid {
	data := []byte(input)
	if prefix == nil {
		c, err := abi.CidBuilder.Sum(data)
		if err != nil {
			panic(err)
		}
		return c
	}
	c, err := prefix.Sum(data)
	switch {
	case errors.Is(err, mh.ErrSumNotSupported):
		// multihash library doesn't support this hash function.
		// just fake it.
	case err == nil:
		return c
	default:
		panic(err)
	}

	sum := sha256.Sum256(data)
	hash, err := mh.Encode(sum[:], prefix.MhType)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(prefix.Codec, hash)
}
