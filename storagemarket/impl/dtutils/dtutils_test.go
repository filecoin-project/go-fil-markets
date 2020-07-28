package dtutils_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

func TestProviderDataTransferSubscriber(t *testing.T) {
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	tests := map[string]struct {
		code          datatransfer.EventCode
		status        datatransfer.Status
		called        bool
		voucher       datatransfer.Voucher
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a storage voucher": {
			called:  false,
			voucher: nil,
		},
		"open event": {
			code:   datatransfer.Open,
			status: datatransfer.Requested,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferInitiated,
		},
		"completion status": {
			code:   datatransfer.Complete,
			status: datatransfer.Completed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferCompleted,
		},
		"error event": {
			code:   datatransfer.Error,
			status: datatransfer.Failed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferFailed,
			expectedArgs:  []interface{}{dtutils.ErrDataTransferFailed},
		},
		"other event": {
			code:   datatransfer.Progress,
			status: datatransfer.Ongoing,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ProviderDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code}, shared_testutil.NewTestChannel(
				shared_testutil.TestChannelParams{Vouchers: []datatransfer.Voucher{data.voucher}, Status: data.status},
			))
			if data.called {
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, data.expectedID)
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				require.False(t, fdg.called)
			}
		})
	}
}

func TestClientDataTransferSubscriber(t *testing.T) {
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	tests := map[string]struct {
		code          datatransfer.EventCode
		status        datatransfer.Status
		called        bool
		voucher       datatransfer.Voucher
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a storage voucher": {
			called:  false,
			voucher: nil,
		},
		"completion event": {
			code:   datatransfer.Complete,
			status: datatransfer.Completed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferComplete,
		},
		"error event": {
			code:   datatransfer.Error,
			status: datatransfer.Failed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferFailed,
			expectedArgs:  []interface{}{dtutils.ErrDataTransferFailed},
		},
		"other event": {
			code:   datatransfer.Progress,
			status: datatransfer.Ongoing,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ClientDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code}, shared_testutil.NewTestChannel(
				shared_testutil.TestChannelParams{Vouchers: []datatransfer.Voucher{data.voucher}, Status: data.status},
			))
			if data.called {
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, data.expectedID)
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				require.False(t, fdg.called)
			}
		})
	}
}

func TestTransportConfigurer(t *testing.T) {
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	expectedChannelID := shared_testutil.MakeTestChannelID()

	testCases := map[string]struct {
		voucher          datatransfer.Voucher
		transport        datatransfer.Transport
		returnedStore    *multistore.Store
		returnedStoreErr error
		getterCalled     bool
		useStoreCalled   bool
	}{
		"non-storage voucher": {
			voucher:      nil,
			getterCalled: false,
		},
		"non-configurable transport": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:    &fakeTransport{},
			getterCalled: false,
		},
		"store getter errors": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   false,
			returnedStore:    nil,
			returnedStoreErr: errors.New("something went wrong"),
		},
		"store getter succeeds": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   true,
			returnedStore:    &multistore.Store{},
			returnedStoreErr: nil,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			storeGetter := &fakeStoreGetter{returnedErr: data.returnedStoreErr, returnedStore: data.returnedStore}
			transportConfigurer := dtutils.TransportConfigurer(storeGetter)
			transportConfigurer(expectedChannelID, data.voucher, data.transport)
			if data.getterCalled {
				require.True(t, storeGetter.called)
				require.Equal(t, expectedProposalCID, storeGetter.lastProposalCid)
				fgt, ok := data.transport.(*fakeGsTransport)
				require.True(t, ok)
				if data.useStoreCalled {
					require.True(t, fgt.called)
					require.Equal(t, expectedChannelID, fgt.lastChannelID)
				} else {
					require.False(t, fgt.called)
				}
			} else {
				require.False(t, storeGetter.called)
			}
		})
	}
}

type fakeDealGroup struct {
	returnedErr error
	called      bool
	lastID      interface{}
	lastEvent   fsm.EventName
	lastArgs    []interface{}
}

func (fdg *fakeDealGroup) Send(id interface{}, name fsm.EventName, args ...interface{}) (err error) {
	fdg.lastID = id
	fdg.lastEvent = name
	fdg.lastArgs = args
	fdg.called = true
	return fdg.returnedErr
}

type fakeStoreGetter struct {
	lastProposalCid cid.Cid
	returnedErr     error
	returnedStore   *multistore.Store
	called          bool
}

func (fsg *fakeStoreGetter) Get(proposalCid cid.Cid) (*multistore.Store, error) {
	fsg.lastProposalCid = proposalCid
	fsg.called = true
	return fsg.returnedStore, fsg.returnedErr
}

type fakeTransport struct{}

func (ft *fakeTransport) OpenChannel(ctx context.Context, dataSender peer.ID, channelID datatransfer.ChannelID, root ipld.Link, stor ipld.Node, msg datatransfer.Message) error {
	return nil
}

func (ft *fakeTransport) CloseChannel(ctx context.Context, chid datatransfer.ChannelID) error {
	return nil
}

func (ft *fakeTransport) SetEventHandler(events datatransfer.EventsHandler) error {
	return nil
}

func (ft *fakeTransport) CleanupChannel(chid datatransfer.ChannelID) {
}

type fakeGsTransport struct {
	datatransfer.Transport
	lastChannelID datatransfer.ChannelID
	lastLoader    ipld.Loader
	lastStorer    ipld.Storer
	called        bool
}

func (fgt *fakeGsTransport) UseStore(channelID datatransfer.ChannelID, loader ipld.Loader, storer ipld.Storer) error {
	fgt.lastChannelID = channelID
	fgt.lastLoader = loader
	fgt.lastStorer = storer
	fgt.called = true
	return nil
}
