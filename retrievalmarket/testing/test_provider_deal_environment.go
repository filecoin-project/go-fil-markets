package testing

import (
	"context"
	"errors"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
)

type TestProviderDealEnvironment struct {
	node                rm.RetrievalProviderNode
	ds                  rmnet.RetrievalDealStream
	nextResponse        int
	responses           []ReadBlockResponse
	expectedParams      map[dealParamsKey]error
	receivedParams      map[dealParamsKey]struct{}
	expectedCIDs        map[cid.Cid]uint64
	expectedMissingCIDs map[cid.Cid]struct{}
	receivedCIDs        map[cid.Cid]struct{}
	receivedMissingCIDs map[cid.Cid]struct{}
}

func NewTestProviderDealEnvironment(node rm.RetrievalProviderNode,
	ds rmnet.RetrievalDealStream,
	responses []ReadBlockResponse) *TestProviderDealEnvironment {
	return &TestProviderDealEnvironment{
		node:                node,
		ds:                  ds,
		nextResponse:        0,
		responses:           responses,
		expectedParams:      make(map[dealParamsKey]error),
		receivedParams:      make(map[dealParamsKey]struct{}),
		expectedCIDs:        make(map[cid.Cid]uint64),
		expectedMissingCIDs: make(map[cid.Cid]struct{}),
		receivedCIDs:        make(map[cid.Cid]struct{}),
		receivedMissingCIDs: make(map[cid.Cid]struct{})}
}

// ExpectPiece records a piece being expected to be queried and return the given piece info
func (te *TestProviderDealEnvironment) ExpectPiece(c cid.Cid, size uint64) {
	te.expectedCIDs[c] = size
}

// ExpectMissingPiece records a piece being expected to be queried and should fail
func (te *TestProviderDealEnvironment) ExpectMissingPiece(c cid.Cid) {
	te.expectedMissingCIDs[c] = struct{}{}
}

func (te *TestProviderDealEnvironment) ExpectParams(pricePerByte abi.TokenAmount,
	paymentInterval uint64,
	paymentIntervalIncrease uint64,
	response error) {
	te.expectedParams[dealParamsKey{pricePerByte.String(), paymentInterval, paymentIntervalIncrease}] = response
}

func (te *TestProviderDealEnvironment) VerifyExpectations(t *testing.T) {
	require.Equal(t, len(te.expectedParams), len(te.receivedParams))
	require.Equal(t, len(te.expectedCIDs), len(te.receivedCIDs))
	require.Equal(t, len(te.expectedMissingCIDs), len(te.receivedMissingCIDs))
}

func (te *TestProviderDealEnvironment) Node() rm.RetrievalProviderNode {
	return te.node
}

func (te *TestProviderDealEnvironment) DealStream(_ rm.ProviderDealIdentifier) rmnet.RetrievalDealStream {
	return te.ds
}

func (te *TestProviderDealEnvironment) GetPieceSize(c cid.Cid) (uint64, error) {
	pio, ok := te.expectedCIDs[c]
	if ok {
		te.receivedCIDs[c] = struct{}{}
		return pio, nil
	}
	_, ok = te.expectedMissingCIDs[c]
	if ok {
		te.receivedMissingCIDs[c] = struct{}{}
		return 0, rm.ErrNotFound
	}
	return 0, errors.New("GetPieceSize failed")
}

func (te *TestProviderDealEnvironment) CheckDealParams(pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) error {
	key := dealParamsKey{pricePerByte.String(), paymentInterval, paymentIntervalIncrease}
	err, ok := te.expectedParams[key]
	if !ok {
		return errors.New("CheckDealParamsFailed")
	}
	te.receivedParams[key] = struct{}{}
	return err
}

func (te *TestProviderDealEnvironment) NextBlock(_ context.Context, _ rm.ProviderDealIdentifier) (rm.Block, bool, error) {
	if te.nextResponse >= len(te.responses) {
		return rm.EmptyBlock, false, errors.New("Something went wrong")
	}
	response := te.responses[te.nextResponse]
	te.nextResponse += 1
	return response.Block, response.Done, response.Err
}

type dealParamsKey struct {
	pricePerByte            string
	paymentInterval         uint64
	paymentIntervalIncrease uint64
}
