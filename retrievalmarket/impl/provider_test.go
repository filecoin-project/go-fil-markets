package retrievalimpl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/filecoin-project/go-address"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestHandleQueryStream(t *testing.T) {

	pcid := []byte(string("applesauce"))
	expectedPeer := peer.ID("somepeer")
	expectedSize := uint64(1234)
	expectedAddress := address.TestAddress2
	expectedPricePerByte := tokenamount.FromInt(4321)
	expectedPaymentInterval := uint64(4567)
	expectedPaymentIntervalIncrease := uint64(100)

	readWriteQueryStream := func() network.RetrievalQueryStream {
		qRead, qWrite := tut.QueryReadWriter()
		qrRead, qrWrite := tut.QueryResponseReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespReader: qrRead,
			RespWriter: qrWrite,
		})
		return qs
	}

	receiveStreamOnProvider := func(qs network.RetrievalQueryStream, node *testRetrievalProviderNode) {
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{})
		c := retrievalimpl.NewProvider(expectedAddress, node, net)
		c.SetPricePerByte(expectedPricePerByte)
		c.SetPaymentInterval(expectedPaymentInterval, expectedPaymentIntervalIncrease)
		_ = c.Start()
		net.ReceiveQueryStream(qs)
	}

	t.Run("it works", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PieceCID: pcid,
		})
		require.NoError(t, err)
		node := newTestRetrievalProviderNode()
		node.expectPiece(pcid, expectedSize)

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.verifyExpectations(t)
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseAvailable)
		require.Equal(t, response.Size, expectedSize)
		require.Equal(t, response.PaymentAddress, expectedAddress)
		require.Equal(t, response.MinPricePerByte, expectedPricePerByte)
		require.Equal(t, response.MaxPaymentInterval, expectedPaymentInterval)
		require.Equal(t, response.MaxPaymentIntervalIncrease, expectedPaymentIntervalIncrease)
	})

	t.Run("piece not found", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PieceCID: pcid,
		})
		require.NoError(t, err)
		node := newTestRetrievalProviderNode()
		node.expectMissingPiece(pcid)

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.verifyExpectations(t)
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseUnavailable)
		require.Equal(t, response.PaymentAddress, expectedAddress)
		require.Equal(t, response.MinPricePerByte, expectedPricePerByte)
		require.Equal(t, response.MaxPaymentInterval, expectedPaymentInterval)
		require.Equal(t, response.MaxPaymentIntervalIncrease, expectedPaymentIntervalIncrease)
	})

	t.Run("error reading piece", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PieceCID: pcid,
		})
		require.NoError(t, err)
		node := newTestRetrievalProviderNode()

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.verifyExpectations(t)
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseError)
		require.NotEmpty(t, response.Message)
	})

	t.Run("when ReadQuery fails", func(t *testing.T) {
		qs := readWriteQueryStream()
		node := newTestRetrievalProviderNode()
		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.verifyExpectations(t)
		require.NotNil(t, err)
		require.Equal(t, response, retrievalmarket.QueryResponseUndefined)
	})

	t.Run("when WriteQueryResponse fails", func(t *testing.T) {
		qRead, qWrite := tut.QueryReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespWriter: tut.FailResponseWriter,
		})
		err := qs.WriteQuery(retrievalmarket.Query{
			PieceCID: pcid,
		})
		require.NoError(t, err)
		node := newTestRetrievalProviderNode()
		node.expectPiece(pcid, expectedSize)

		receiveStreamOnProvider(qs, node)

		node.verifyExpectations(t)
	})
}

type testRetrievalProviderNode struct {
	expectedPieces        map[string]uint64
	expectedMissingPieces map[string]struct{}
	receivedPiecesSizes   map[string]struct{}
	receivedMissingPieces map[string]struct{}
}

func newTestRetrievalProviderNode() *testRetrievalProviderNode {
	return &testRetrievalProviderNode{
		expectedPieces:        make(map[string]uint64),
		expectedMissingPieces: make(map[string]struct{}),
		receivedPiecesSizes:   make(map[string]struct{}),
		receivedMissingPieces: make(map[string]struct{}),
	}
}

func (trpn *testRetrievalProviderNode) expectPiece(pieceCid []byte, size uint64) {
	trpn.expectedPieces[string(pieceCid)] = size
}

func (trpn *testRetrievalProviderNode) expectMissingPiece(pieceCid []byte) {
	trpn.expectedMissingPieces[string(pieceCid)] = struct{}{}
}

func (trpn *testRetrievalProviderNode) verifyExpectations(t *testing.T) {
	require.Equal(t, len(trpn.expectedPieces), len(trpn.receivedPiecesSizes))
	require.Equal(t, len(trpn.expectedMissingPieces), len(trpn.receivedMissingPieces))

}

func (trpn *testRetrievalProviderNode) GetPieceSize(pieceCid []byte) (uint64, error) {
	size, ok := trpn.expectedPieces[string(pieceCid)]
	if ok {
		trpn.receivedPiecesSizes[string(pieceCid)] = struct{}{}
		return size, nil
	}
	_, ok = trpn.expectedMissingPieces[string(pieceCid)]
	if ok {
		trpn.receivedMissingPieces[string(pieceCid)] = struct{}{}
		return 0, retrievalmarket.ErrNotFound
	}
	return 0, errors.New("Something went wrong")
}

func (trpn *testRetrievalProviderNode) SealedBlockstore(approveUnseal func() error) blockstore.Blockstore {
	panic("not implemented")
}

func (trpn *testRetrievalProviderNode) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *types.SignedVoucher, proof []byte, expectedAmount tokenamount.TokenAmount) (tokenamount.TokenAmount, error) {
	panic("not implemented")
}
