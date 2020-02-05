package retrievalimpl_test

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/specs-actors/actors/abi"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestHandleQueryStream(t *testing.T) {

	pcid := tut.GenerateCids(1)[0]
	expectedPeer := peer.ID("somepeer")
	expectedSize := uint64(1234)
	expectedPieceCID := []byte("applesauce")
	expectedCIDInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPieceCID,
			},
		},
	}
	expectedPiece := piecestore.PieceInfo{
		Deals: []piecestore.DealInfo{
			piecestore.DealInfo{
				Length: expectedSize,
			},
		},
	}
	expectedAddress := address.TestAddress2
	expectedPricePerByte := abi.NewTokenAmount(4321)
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

	receiveStreamOnProvider := func(qs network.RetrievalQueryStream, pieceStore piecestore.PieceStore) {
		node := testnodes.NewTestRetrievalProviderNode()
		bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{})
		c := retrievalimpl.NewProvider(expectedAddress, node, net, pieceStore, bs)
		c.SetPricePerByte(expectedPricePerByte)
		c.SetPaymentInterval(expectedPaymentInterval, expectedPaymentIntervalIncrease)
		_ = c.Start()
		net.ReceiveQueryStream(qs)
	}

	t.Run("it works", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: pcid,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()

		pieceStore.ExpectCID(pcid, expectedCIDInfo)
		pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)

		receiveStreamOnProvider(qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		pieceStore.VerifyExpectations(t)
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
			PayloadCID: pcid,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		pieceStore.ExpectCID(pcid, expectedCIDInfo)
		pieceStore.ExpectMissingPiece(expectedPieceCID)

		receiveStreamOnProvider(qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		pieceStore.VerifyExpectations(t)
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseUnavailable)
		require.Equal(t, response.PaymentAddress, expectedAddress)
		require.Equal(t, response.MinPricePerByte, expectedPricePerByte)
		require.Equal(t, response.MaxPaymentInterval, expectedPaymentInterval)
		require.Equal(t, response.MaxPaymentIntervalIncrease, expectedPaymentIntervalIncrease)
	})

	t.Run("cid info not found", func(t *testing.T) {
		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: pcid,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		pieceStore.ExpectMissingCID(pcid)

		receiveStreamOnProvider(qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		pieceStore.VerifyExpectations(t)
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
			PayloadCID: pcid,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()

		receiveStreamOnProvider(qs, pieceStore)

		response, err := qs.ReadQueryResponse()
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseError)
		require.NotEmpty(t, response.Message)
	})

	t.Run("when ReadQuery fails", func(t *testing.T) {
		qs := readWriteQueryStream()
		pieceStore := tut.NewTestPieceStore()

		receiveStreamOnProvider(qs, pieceStore)

		response, err := qs.ReadQueryResponse()
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
			PayloadCID: pcid,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		pieceStore.ExpectCID(pcid, expectedCIDInfo)
		pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)

		receiveStreamOnProvider(qs, pieceStore)

		pieceStore.VerifyExpectations(t)
	})
}
