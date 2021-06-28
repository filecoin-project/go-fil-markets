// Package clientutils provides utility functions for the storage client & client FSM
package clientutils

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/multiformats/go-multibase"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/writer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// CommP calculates the commP for a given dataref
// In Markets, CommP = PieceCid.
// We can't rely on the CARv1 payload in the given CARv2 file being deterministic as the client could have
// written a "non-deterministic/unordered" CARv2 file.
// So, we need to do a CARv1 traversal here by giving the traverser a random access CARv2 blockstore that wraps the given CARv2 file.
func CommP(ctx context.Context, CARv2FilePath string, data *storagemarket.DataRef) (cid.Cid, abi.UnpaddedPieceSize, error) {
	// if we already have the PieceCid, there's no need to do anything here.
	if data.PieceCid != nil {
		return *data.PieceCid, data.PieceSize, nil
	}

	// It's an error if we don't already have the PieceCid for an offline deal i.e. manual transfer.
	if data.TransferType == storagemarket.TTManual {
		return cid.Undef, 0, xerrors.New("Piece CID and size must be set for manual transfer")
	}

	dataCIDSize, err := CommPFromCARV2(ctx, CARv2FilePath, data.Root)
	if err != nil {
		return cid.Undef, 0, err
	}
	return dataCIDSize.PieceCID, dataCIDSize.PieceSize.Unpadded(), nil
}

func CommPFromCARV2(ctx context.Context, CARv2FilePath string, root cid.Cid) (writer.DataCIDSize, error) {
	if CARv2FilePath == "" {
		return writer.DataCIDSize{}, xerrors.New("need Carv2 file path to get a read-only blockstore")
	}

	rdOnly, err := blockstore.OpenReadOnly(CARv2FilePath, true)
	if err != nil {
		return writer.DataCIDSize{}, xerrors.Errorf("failed to open read-only blockstore: %w", err)
	}
	defer rdOnly.Close()

	// do a CARv1 traversal with the DFS selector.
	sc := car.NewSelectiveCar(ctx, rdOnly, []car.Dag{{Root: root, Selector: shared.AllSelector()}})
	prepared, err := sc.Prepare()
	if err != nil {
		return writer.DataCIDSize{}, xerrors.Errorf("failed to prepare CAR: %w", err)
	}

	// write out the deterministic CARv1 payload to the CommP writer and calculate the CommP.
	commpWriter := &writer.Writer{}
	err = prepared.Dump(commpWriter)
	if err != nil {
		return writer.DataCIDSize{}, xerrors.Errorf("failed to write CARv1 to commP writer: %w", err)
	}
	dataCIDSize, err := commpWriter.Sum()
	if err != nil {
		return writer.DataCIDSize{}, xerrors.Errorf("commpWriter.Sum failed: %w", err)
	}

	return dataCIDSize, nil
}

// VerifyFunc is a function that can validate a signature for a given address and bytes
type VerifyFunc func(context.Context, crypto.Signature, address.Address, []byte, shared.TipSetToken) (bool, error)

// VerifyResponse verifies the signature on the given signed response matches
// the given miner address, using the given signature verification function
func VerifyResponse(ctx context.Context, resp network.SignedResponse, minerAddr address.Address, tok shared.TipSetToken, verifier VerifyFunc) error {
	b, err := cborutil.Dump(&resp.Response)
	if err != nil {
		return err
	}
	verified, err := verifier(ctx, *resp.Signature, minerAddr, b, tok)
	if err != nil {
		return err
	}

	if !verified {
		return xerrors.New("could not verify signature")
	}

	return nil
}

// LabelField makes a label field for a deal proposal as a multibase encoding
// of the payload CID (B58BTC for V0, B64 for V1)
//
func LabelField(payloadCID cid.Cid) (string, error) {
	if payloadCID.Version() == 0 {
		return payloadCID.StringOfBase(multibase.Base58BTC)
	}
	return payloadCID.StringOfBase(multibase.Base64)
}
