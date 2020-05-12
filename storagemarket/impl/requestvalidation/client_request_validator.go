package requestvalidation

import (
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
)

var _ datatransfer.RequestValidator = &ClientRequestValidator{}

// ClientRequestValidator validates data transfer requests for the client
// in a storage market
type ClientRequestValidator struct {
	deals *statestore.StateStore
}

// NewClientRequestValidator returns a new client request validator for the
// given datastore
func NewClientRequestValidator(deals *statestore.StateStore) *ClientRequestValidator {
	crv := &ClientRequestValidator{
		deals: deals,
	}
	return crv
}

// ValidatePush validates a push request received from the peer that will send data
// Will always error because clients should not accept push requests from a provider
// in a storage deal (i.e. send data to client).
func (c *ClientRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ErrNoPushAccepted
}

// ValidatePull validates a pull request received from the peer that will receive data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the receiver (miner)
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
func (c *ClientRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ValidatePull(c.deals, receiver, voucher, baseCid, Selector)
}
