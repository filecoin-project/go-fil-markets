package requestvalidation

import (
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
)

type UnifiedRequestValidator struct {
	pushDeals *statestore.StateStore
	pullDeals *statestore.StateStore
}

func NewUnifiedRequestValidator(pushDeals *statestore.StateStore, pullDeals *statestore.StateStore) *UnifiedRequestValidator {
	return &UnifiedRequestValidator{
		pushDeals: pushDeals,
		pullDeals: pullDeals,
	}
}

func (v *UnifiedRequestValidator) SetPushDeals(pushDeals *statestore.StateStore) {
	v.pushDeals = pushDeals
}

func (v *UnifiedRequestValidator) SetAcceptPulls(pullDeals *statestore.StateStore) {
	v.pullDeals = pullDeals
}

func (v *UnifiedRequestValidator) ValidatePush(sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	if v.pushDeals == nil {
		return ErrNoPushAccepted
	}

	return ValidatePush(v.pushDeals, sender, voucher, baseCid, selector)
}

func (v *UnifiedRequestValidator) ValidatePull(receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	if v.pullDeals == nil {
		return ErrNoPullAccepted
	}

	return ValidatePull(v.pullDeals, receiver, voucher, baseCid, selector)
}

var _ datatransfer.RequestValidator = &UnifiedRequestValidator{}
