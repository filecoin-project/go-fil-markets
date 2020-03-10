package providerutils

import (
	"context"
	"errors"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

var log = logging.Logger("storagemarket_impl")
var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

type VerifyFunc func(crypto.Signature, address.Address, []byte) bool

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

type WorkerLookupFunc func(context.Context, address.Address) (address.Address, error)
type SignFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)

func SignMinerData(data interface{}, ctx context.Context, address address.Address, workerLookup WorkerLookupFunc, sign SignFunc) (*crypto.Signature, error) {
	msg, err := cborutil.Dump(data)
	if err != nil {
		return nil, xerrors.Errorf("serializing: %w", err)
	}

	worker, err := workerLookup(ctx, address)
	if err != nil {
		return nil, err
	}

	sig, err := sign(ctx, worker, msg)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign: %w", err)
	}
	return sig, nil
}

// DataTransferSubscribe is the function called when an event occurs in a data
// transfer -- it reads the voucher to verify this even occurred in a storage
// market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func DataTransferSubscriber(deals fsm.Group) datatransfer.Subscriber {
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
