package providerutils

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-fil-markets/shared"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

var log = logging.Logger("storagemarket_impl")
var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

// VerifyFunc is a function that can validate a signature for a given address and bytes
type VerifyFunc func(crypto.Signature, address.Address, []byte) bool

// VerifyProposal verifies the signature on the given signed proposal matches
// the client addres for the proposal, using the given signature verification function
func VerifyProposal(sdp market.ClientDealProposal, verifier VerifyFunc) error {
	b, err := cborutil.Dump(&sdp.Proposal)
	if err != nil {
		return err
	}
	verified := verifier(sdp.ClientSignature, sdp.Proposal.Client, b)
	if !verified {
		return xerrors.New("could not verify signature")
	}
	return nil
}

// WorkerLookupFunc is a function that can lookup a miner worker address from a storage miner actor
type WorkerLookupFunc func(context.Context, address.Address, shared.TipSetToken) (address.Address, error)

// SignFunc is a function that can sign a set of bytes with a given address
type SignFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)

// SignMinerData signs the given data structure with a signature for the given address
func SignMinerData(ctx context.Context, data interface{}, address address.Address, tok shared.TipSetToken, workerLookup WorkerLookupFunc, sign SignFunc) (*crypto.Signature, error) {
	msg, err := cborutil.Dump(data)
	if err != nil {
		return nil, xerrors.Errorf("serializing: %w", err)
	}

	worker, err := workerLookup(ctx, address, tok)
	if err != nil {
		return nil, err
	}

	sig, err := sign(ctx, worker, msg)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign: %w", err)
	}
	return sig, nil
}

// EventReceiver is any thing that can receive FSM events
type EventReceiver interface {
	Send(id interface{}, name fsm.EventName, args ...interface{}) (err error)
}

// DataTransferSubscriber is the function called when an event occurs in a data
// transfer -- it reads the voucher to verify this even occurred in a storage
// market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func DataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		voucher, ok := channelState.Voucher().(*requestvalidation.StorageDataTransferVoucher)
		// if this event is for a transfer not related to storage, ignore
		if !ok {
			return
		}

		// data transfer events for opening and progress do not affect deal state
		switch event.Code {
		case datatransfer.Complete:
			err := deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferCompleted)
			if err != nil {
				log.Errorf("processing dt event: %w", err)
			}
		case datatransfer.Error:
			err := deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferFailed, ErrDataTransferFailed)
			if err != nil {
				log.Errorf("processing dt event: %w", err)
			}
		default:
		}
	}
}
