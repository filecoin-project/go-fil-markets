// Package testing provides test implementations of retieval market interfaces
package testing

import (
	"context"
	"io"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
)

// TestProviderDealEnvironment is a test implementation of ProviderDealEnvironment used
// by the provider state machine.
type TestProviderDealEnvironment struct {
	node                    rm.RetrievalProviderNode
	ResumeDataTransferError error
	ReadIntoBlockstoreError error
	TrackTransferError      error
	UntrackTransferError    error
	CloseDataTransferError  error
	DeleteStoreError        error
}

// NewTestProviderDealEnvironment returns a new TestProviderDealEnvironment instance
func NewTestProviderDealEnvironment(node rm.RetrievalProviderNode) *TestProviderDealEnvironment {
	return &TestProviderDealEnvironment{
		node: node,
	}
}

// Node returns a provider node instance
func (te *TestProviderDealEnvironment) Node() rm.RetrievalProviderNode {
	return te.node
}

func (te *TestProviderDealEnvironment) DeleteStore(storeID multistore.StoreID) error {
	return te.DeleteStoreError
}

func (te *TestProviderDealEnvironment) ReadIntoBlockstore(storeID multistore.StoreID, pieceData io.ReadCloser) error {
	return te.ReadIntoBlockstoreError
}

func (te *TestProviderDealEnvironment) TrackTransfer(deal rm.ProviderDealState) error {
	return te.TrackTransferError
}

func (te *TestProviderDealEnvironment) UntrackTransfer(deal rm.ProviderDealState) error {
	return te.UntrackTransferError
}

func (te *TestProviderDealEnvironment) ResumeDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return te.ResumeDataTransferError
}

func (te *TestProviderDealEnvironment) CloseDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return te.CloseDataTransferError
}

// TrivialTestDecider is a shortest possible DealDecider that accepts all deals
var TrivialTestDecider retrievalimpl.DealDecider = func(_ context.Context, _ rm.ProviderDealState) (bool, string, error) {
	return true, "", nil
}
