package providerutils_test

import (
	"context"
	"errors"
	"testing"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

func TestVerifyProposal(t *testing.T) {
	tests := map[string]struct {
		proposal  market.ClientDealProposal
		verifier  providerutils.VerifyFunc
		shouldErr bool
	}{
		"successful verification": {
			proposal:  *shared_testutil.MakeTestClientDealProposal(),
			verifier:  func(crypto.Signature, address.Address, []byte) bool { return true },
			shouldErr: false,
		},
		"bad proposal": {
			proposal: market.ClientDealProposal{
				Proposal:        market.DealProposal{},
				ClientSignature: *shared_testutil.MakeTestSignature(),
			},
			verifier:  func(crypto.Signature, address.Address, []byte) bool { return true },
			shouldErr: true,
		},
		"verification fails": {
			proposal:  *shared_testutil.MakeTestClientDealProposal(),
			verifier:  func(crypto.Signature, address.Address, []byte) bool { return false },
			shouldErr: true,
		},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			err := providerutils.VerifyProposal(data.proposal, data.verifier)
			require.Equal(t, err != nil, data.shouldErr)
		})
	}
}

func TestSignMinerData(t *testing.T) {
	ctx := context.Background()
	successLookup := func(context.Context, address.Address) (address.Address, error) {
		return address.TestAddress2, nil
	}
	successSign := func(context.Context, address.Address, []byte) (*crypto.Signature, error) {
		return shared_testutil.MakeTestSignature(), nil
	}
	tests := map[string]struct {
		data         interface{}
		workerLookup providerutils.WorkerLookupFunc
		signBytes    providerutils.SignFunc
		shouldErr    bool
	}{
		"succeeds": {
			data:         shared_testutil.MakeTestStorageAsk(),
			workerLookup: successLookup,
			signBytes:    successSign,
			shouldErr:    false,
		},
		"cbor dump errors": {
			data:         &network.Response{},
			workerLookup: successLookup,
			signBytes:    successSign,
			shouldErr:    true,
		},
		"worker lookup errors": {
			data: shared_testutil.MakeTestStorageAsk(),
			workerLookup: func(context.Context, address.Address) (address.Address, error) {
				return address.Undef, errors.New("Something went wrong")
			},
			signBytes: successSign,
			shouldErr: true,
		},
		"signing errors": {
			data:         shared_testutil.MakeTestStorageAsk(),
			workerLookup: successLookup,
			signBytes: func(context.Context, address.Address, []byte) (*crypto.Signature, error) {
				return nil, errors.New("something went wrong")
			},
			shouldErr: true,
		},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := providerutils.SignMinerData(ctx, data.data, address.TestAddress, data.workerLookup, data.signBytes)
			require.Equal(t, err != nil, data.shouldErr)
		})
	}
}

func TestDataTransferSubscriber(t *testing.T) {
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	tests := map[string]struct {
		code          datatransfer.EventCode
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
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferCompleted,
		},
		"error event": {
			code:   datatransfer.Error,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferFailed,
			expectedArgs:  []interface{}{providerutils.ErrDataTransferFailed},
		},
		"other event": {
			code:   datatransfer.Progress,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := providerutils.DataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code}, datatransfer.ChannelState{
				Channel: datatransfer.NewChannel(datatransfer.TransferID(0), cid.Undef, nil, data.voucher, peer.ID(""), peer.ID(""), 0),
			})
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
