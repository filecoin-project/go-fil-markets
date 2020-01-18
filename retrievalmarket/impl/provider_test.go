package retrievalimpl_test

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
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

	receiveStreamOnProvider := func(qs network.RetrievalQueryStream, node *testnodes.TestRetrievalProviderNode) {
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
		// node := newTestRetrievalProviderNode(nil)
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectPiece(pcid, expectedSize)

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.VerifyExpectations(t)
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
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectMissingPiece(pcid)

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.VerifyExpectations(t)
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
		node := testnodes.NewTestRetrievalProviderNode()

		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.VerifyExpectations(t)
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseError)
		require.NotEmpty(t, response.Message)
	})

	t.Run("when ReadQuery fails", func(t *testing.T) {
		qs := readWriteQueryStream()
		node := testnodes.NewTestRetrievalProviderNode()
		receiveStreamOnProvider(qs, node)

		response, err := qs.ReadQueryResponse()
		node.VerifyExpectations(t)
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
		node := testnodes.NewTestRetrievalProviderNode()
		node.ExpectPiece(pcid, expectedSize)

		receiveStreamOnProvider(qs, node)

		node.VerifyExpectations(t)
	})
}
