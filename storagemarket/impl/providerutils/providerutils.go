package providerutils

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/blockrecorder"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

var log = logging.Logger("storagemarket_impl")
var (
	// ErrDataTransferFailed means a data transfer for a deal failed
	ErrDataTransferFailed = errors.New("deal data transfer failed")
)

// VerifyFunc is a function that can validate a signature for a given address and bytes
type VerifyFunc func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error)

// VerifyProposal verifies the signature on the given signed proposal matches
// the client addres for the proposal, using the given signature verification function
func VerifyProposal(ctx context.Context, sdp market.ClientDealProposal, tok shared.TipSetToken, verifier VerifyFunc) error {
	b, err := cborutil.Dump(&sdp.Proposal)
	if err != nil {
		return err
	}

	verified, err := verifier(ctx, sdp.ClientSignature, sdp.Proposal.Client, b, tok)
	if err != nil {
		return xerrors.Errorf("verifying: %w", err)
	}

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

// CommPGenerator is a commP generating function that writes to a file
type CommPGenerator func(abi.RegisteredProof, cid.Cid, ipld.Node, ...car.OnNewCarBlockFunc) (cid.Cid, filestore.Path, abi.UnpaddedPieceSize, error)

// GeneratePieceCommitmentWithMetadata generates a piece commitment along with block metadata
func GeneratePieceCommitmentWithMetadata(
	fileStore filestore.FileStore,
	commPGenerator CommPGenerator,
	proofType abi.RegisteredProof,
	payloadCid cid.Cid,
	selector ipld.Node) (cid.Cid, filestore.Path, filestore.Path, error) {
	metadataFile, err := fileStore.CreateTemp()
	if err != nil {
		return cid.Cid{}, "", "", err
	}
	blockRecorder := blockrecorder.RecordEachBlockTo(metadataFile)
	pieceCid, path, _, err := commPGenerator(proofType, payloadCid, selector, blockRecorder)
	_ = metadataFile.Close()
	if err != nil {
		_ = fileStore.Delete(metadataFile.Path())
		return cid.Cid{}, "", "", err
	}
	return pieceCid, path, metadataFile.Path(), err
}

// LoadBlockLocations loads a metadata file then converts it to a map of cid -> blockLocation
func LoadBlockLocations(fs filestore.FileStore, metadataPath filestore.Path) (map[cid.Cid]piecestore.BlockLocation, error) {
	metadataFile, err := fs.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	metadata, err := blockrecorder.ReadBlockMetadata(metadataFile)
	_ = metadataFile.Close()
	if err != nil {
		return nil, err
	}
	blockLocations := make(map[cid.Cid]piecestore.BlockLocation, len(metadata))
	for _, metadatum := range metadata {
		blockLocations[metadatum.CID] = piecestore.BlockLocation{RelOffset: metadatum.Offset, BlockSize: metadatum.Size}
	}
	return blockLocations, nil
}
