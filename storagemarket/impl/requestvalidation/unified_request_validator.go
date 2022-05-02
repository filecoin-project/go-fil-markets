package requestvalidation

import (
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// PushDeals gets deal states for Push validations
type PushDeals interface {
	Get(cid.Cid) (storagemarket.MinerDeal, error)
}

// PullDeals gets deal states for Pull validations
type PullDeals interface {
	Get(cid.Cid) (storagemarket.ClientDeal, error)
}

// UnifiedRequestValidator is a data transfer request validator that validates
// StorageDataTransferVoucher from the given state store
// It can be made to only accept push requests (Provider) or pull requests (Client)
// by passing nil for the statestore value for pushes or pulls
type UnifiedRequestValidator struct {
	pushDeals PushDeals
	pullDeals PullDeals
}

// NewUnifiedRequestValidator returns a new instance of UnifiedRequestValidator
func NewUnifiedRequestValidator(pushDeals PushDeals, pullDeals PullDeals) *UnifiedRequestValidator {
	return &UnifiedRequestValidator{
		pushDeals: pushDeals,
		pullDeals: pullDeals,
	}
}

// SetPushDeals sets the store to look up push deals with
func (v *UnifiedRequestValidator) SetPushDeals(pushDeals PushDeals) {
	v.pushDeals = pushDeals
}

// SetPullDeals sets the store to look up pull deals with
func (v *UnifiedRequestValidator) SetPullDeals(pullDeals PullDeals) {
	v.pullDeals = pullDeals
}

// ValidatePush implements the ValidatePush method of a data transfer request validator.
// If no pushStore exists, it rejects the request
// Otherwise, it calls the ValidatePush function to validate the deal
func (v *UnifiedRequestValidator) ValidatePush(_ datatransfer.ChannelID, sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ValidationResult, error) {
	if v.pushDeals == nil {
		return datatransfer.ValidationResult{}, ErrNoPushAccepted
	}

	err := ValidatePush(v.pushDeals, sender, voucher, baseCid, selector)
	if err != nil {
		return datatransfer.ValidationResult{Accepted: false}, nil
	}
	return datatransfer.ValidationResult{Accepted: true}, nil
}

// ValidatePull implements the ValidatePull method of a data transfer request validator.
// If no pullStore exists, it rejects the request
// Otherwise, it calls the ValidatePull function to validate the deal
func (v *UnifiedRequestValidator) ValidatePull(_ datatransfer.ChannelID, receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ValidationResult, error) {
	if v.pullDeals == nil {
		return datatransfer.ValidationResult{}, ErrNoPullAccepted
	}

	err := ValidatePull(v.pullDeals, receiver, voucher, baseCid, selector)
	if err != nil {
		return datatransfer.ValidationResult{Accepted: false}, nil
	}
	return datatransfer.ValidationResult{Accepted: true}, nil
}

func (v *UnifiedRequestValidator) ValidateRestart(chid datatransfer.ChannelID, channelState datatransfer.ChannelState) (datatransfer.ValidationResult, error) {
	if channelState.IsPull() {
		return v.ValidatePull(chid, channelState.Recipient(), channelState.Voucher(), channelState.BaseCID(), channelState.Selector())
	} else {
		return v.ValidatePush(chid, channelState.Sender(), channelState.Voucher(), channelState.BaseCID(), channelState.Selector())
	}
}

var _ datatransfer.RequestValidator = &UnifiedRequestValidator{}
