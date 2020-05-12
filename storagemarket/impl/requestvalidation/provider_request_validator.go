package requestvalidation

import (
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
)

var _ datatransfer.RequestValidator = &ProviderRequestValidator{}

// ProviderRequestValidator validates data transfer requests for the provider
// in a storage market
type ProviderRequestValidator struct {
	deals *statestore.StateStore
}

// NewProviderRequestValidator returns a new client request validator for the
// given datastore
func NewProviderRequestValidator(deals *statestore.StateStore) *ProviderRequestValidator {
	return &ProviderRequestValidator{
		deals: deals,
	}
}

// ValidatePush validates a push request received from the peer that will send data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the client
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
// TODO: maybe this should accept a dataref?
func (m *ProviderRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ValidatePush(m.deals, sender, voucher, baseCid, Selector)
}

// ValidatePull validates a pull request received from the peer that will receive data.
// Will always error because providers should not accept pull requests from a client
// in a storage deal (i.e. send data to client).
func (m *ProviderRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ErrNoPullAccepted
}
