package shared

import "github.com/filecoin-project/specs-actors/actors/abi"

// TipSetToken is the implementation-nonspecific identity for a tipset.
type TipSetToken []byte

type StateKey interface {
	TipSetToken() TipSetToken
	Height() abi.ChainEpoch
}
