package shared_testutil

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/libp2p/go-libp2p/core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
)

// RegisteredVoucherType records a voucher typed that was registered
type RegisteredVoucherType struct {
	VoucherType datatransfer.TypeIdentifier
	Validator   datatransfer.RequestValidator
}

// RegisteredTransportConfigurer records transport configurer registered for a voucher type
type RegisteredTransportConfigurer struct {
	VoucherType datatransfer.TypeIdentifier
	Configurer  datatransfer.TransportConfigurer
}

// TestDataTransfer is a mock implementation of the data transfer libary
// Most of its functions have no effect
type TestDataTransfer struct {
	RegisteredVoucherTypes         []RegisteredVoucherType
	RegisteredTransportConfigurers []RegisteredTransportConfigurer
	Subscribers                    []datatransfer.Subscriber
}

// NewTestDataTransfer returns a new test interface implementation of datatransfer.Manager
func NewTestDataTransfer() *TestDataTransfer {
	return &TestDataTransfer{}
}

// Start does nothing
func (tdt *TestDataTransfer) Start(ctx context.Context) error {
	return nil
}

// Stop does nothing
func (tdt *TestDataTransfer) Stop(context.Context) error {
	return nil
}

// RegisterVoucherType records the registred voucher type
func (tdt *TestDataTransfer) RegisterVoucherType(voucherType datatransfer.TypeIdentifier, validator datatransfer.RequestValidator) error {
	tdt.RegisteredVoucherTypes = append(tdt.RegisteredVoucherTypes, RegisteredVoucherType{voucherType, validator})
	return nil
}

// RegisterTransportConfigurer records the registered transport configurer
func (tdt *TestDataTransfer) RegisterTransportConfigurer(voucherType datatransfer.TypeIdentifier, configurer datatransfer.TransportConfigurer) error {
	tdt.RegisteredTransportConfigurers = append(tdt.RegisteredTransportConfigurers, RegisteredTransportConfigurer{voucherType, configurer})
	return nil
}

// OpenPushDataChannel does nothing
func (tdt *TestDataTransfer) OpenPushDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.TypedVoucher, baseCid cid.Cid, selector datamodel.Node, options ...datatransfer.TransferOption) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

func (tdt *TestDataTransfer) RestartDataTransferChannel(ctx context.Context, chId datatransfer.ChannelID) error {
	return nil
}

// OpenPullDataChannel does nothing
func (tdt *TestDataTransfer) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.TypedVoucher, baseCid cid.Cid, selector datamodel.Node, options ...datatransfer.TransferOption) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

// SendVoucher does nothing
func (tdt *TestDataTransfer) SendVoucher(ctx context.Context, chid datatransfer.ChannelID, voucher datatransfer.TypedVoucher) error {
	return nil
}

// SendVoucherResult does nothing
func (tdt *TestDataTransfer) SendVoucherResult(ctx context.Context, chid datatransfer.ChannelID, voucherResult datatransfer.TypedVoucher) error {
	return nil
}

// UpdateValidationStatus does nothing
func (tdt *TestDataTransfer) UpdateValidationStatus(ctx context.Context, chid datatransfer.ChannelID, result datatransfer.ValidationResult) error {
	return nil
}

// CloseDataTransferChannel does nothing
func (tdt *TestDataTransfer) CloseDataTransferChannel(ctx context.Context, chid datatransfer.ChannelID) error {
	return nil
}

// PauseDataTransferChannel does nothing
func (tdt *TestDataTransfer) PauseDataTransferChannel(ctx context.Context, chid datatransfer.ChannelID) error {
	return nil
}

// ResumeDataTransferChannel does nothing
func (tdt *TestDataTransfer) ResumeDataTransferChannel(ctx context.Context, chid datatransfer.ChannelID) error {
	return nil
}

// TransferChannelStatus returns ChannelNotFoundError
func (tdt *TestDataTransfer) TransferChannelStatus(ctx context.Context, x datatransfer.ChannelID) datatransfer.Status {
	return datatransfer.ChannelNotFoundError
}

func (tdt *TestDataTransfer) ChannelState(ctx context.Context, chid datatransfer.ChannelID) (datatransfer.ChannelState, error) {
	return nil, nil
}

// SubscribeToEvents records subscribers
func (tdt *TestDataTransfer) SubscribeToEvents(subscriber datatransfer.Subscriber) datatransfer.Unsubscribe {
	tdt.Subscribers = append(tdt.Subscribers, subscriber)
	return func() {}
}

// InProgressChannels returns empty
func (tdt *TestDataTransfer) InProgressChannels(ctx context.Context) (map[datatransfer.ChannelID]datatransfer.ChannelState, error) {
	return map[datatransfer.ChannelID]datatransfer.ChannelState{}, nil
}

func (tdt *TestDataTransfer) OnReady(f datatransfer.ReadyFunc) {

}

var _ datatransfer.Manager = new(TestDataTransfer)
