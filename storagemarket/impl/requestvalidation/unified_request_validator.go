package requestvalidation

import (
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
)

type UnifiedRequestValidator struct {
	acceptsPushes bool
	acceptsPulls  bool
	deals         *statestore.StateStore
}

func NewUnifiedRequestValidator(acceptsPushes bool, acceptsPulls bool, deals *statestore.StateStore) *UnifiedRequestValidator {
	return &UnifiedRequestValidator{
		acceptsPushes: acceptsPushes,
		acceptsPulls:  acceptsPulls,
		deals:         deals,
	}
}

func (v *UnifiedRequestValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	if !v.acceptsPushes {
		return ErrNoPushAccepted
	}

	return ValidatePush(v.deals, sender, voucher, baseCid, selector)
}

func (v *UnifiedRequestValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	if !v.acceptsPulls {
		return ErrNoPullAccepted
	}

	return ValidatePull(v.deals, receiver, voucher, baseCid, selector)
}

var _ datatransfer.RequestValidator = &UnifiedRequestValidator{}
