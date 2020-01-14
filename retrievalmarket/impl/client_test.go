package retrievalimpl_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestClient_Query(t *testing.T) {
	ctx := context.Background()

	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	pcid := []byte(string("applesauce"))
	expectedPeer := peer.ID("somevalue")
	rpeer := retrievalmarket.RetrievalPeer{
		Address: address.TestAddress2,
		ID:      expectedPeer,
	}

	expectedQuery := retrievalmarket.Query{
		PieceCID: pcid,
	}

	expectedQueryResponse := retrievalmarket.QueryResponse{
		Status:                     retrievalmarket.QueryResponseAvailable,
		Size:                       1234,
		PaymentAddress:             address.TestAddress,
		MinPricePerByte:            tokenamount.FromInt(5678),
		MaxPaymentInterval:         4321,
		MaxPaymentIntervalIncrease: 0,
	}

	t.Run("it works", func(t *testing.T) {
		var qsb tut.QueryStreamBuilder = func(p peer.ID) (rmnet.RetrievalQueryStream, error) {
			return tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
				Writer:     tut.ExpectQueryWriter(t, expectedQuery, "queries should match"),
				RespReader: tut.StubbedQueryResponseReader(expectedQueryResponse),
			}), nil
		}
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			QueryStreamBuilder: tut.ExpectPeerOnQueryStreamBuilder(t, expectedPeer, qsb, "Peers should match"),
		})
		c := retrievalimpl.NewClient(net, bs, testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{}))
		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testPeerResolver{})

		resp, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, expectedQueryResponse, resp)
	})

	t.Run("when the stream returns error, returns error", func(t *testing.T) {
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			QueryStreamBuilder: tut.FailNewQueryStream,
		})
		c := retrievalimpl.NewClient(net, bs, testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{}))
		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testPeerResolver{})

		_, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		assert.EqualError(t, err, "new query stream failed")
	})

	t.Run("when WriteQuery fails, returns error", func(t *testing.T) {

		qsbuilder := func(p peer.ID) (network.RetrievalQueryStream, error) {
			newStream := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
				PeerID: p,
				Writer: tut.FailQueryWriter,
			})
			return newStream, nil
		}

		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			QueryStreamBuilder: qsbuilder,
		})
		c := retrievalimpl.NewClient(net, bs, testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{}))
		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testPeerResolver{})

		statusCode, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		assert.EqualError(t, err, "write query failed")
		assert.Equal(t, retrievalmarket.QueryResponseUndefined, statusCode)
	})

	t.Run("when ReadQueryResponse fails, returns error", func(t *testing.T) {
		qsbuilder := func(p peer.ID) (network.RetrievalQueryStream, error) {
			newStream := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
				PeerID:     p,
				RespReader: tut.FailResponseReader,
			})
			return newStream, nil
		}
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			QueryStreamBuilder: qsbuilder,
		})
		c := retrievalimpl.NewClient(
			net,
			bs,
			testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{}),
			&testPeerResolver{})

		statusCode, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		assert.EqualError(t, err, "query response failed")
		assert.Equal(t, retrievalmarket.QueryResponseUndefined, statusCode)
	})
}

func TestClient_FindProviders(t *testing.T) {
	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	expectedPeer := peer.ID("somevalue")

	var qsb tut.QueryStreamBuilder = func(p peer.ID) (rmnet.RetrievalQueryStream, error) {
		return tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			Writer:     tut.TrivialQueryWriter,
			RespReader: tut.TrivialQueryResponseReader,
		}), nil
	}
	net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
		QueryStreamBuilder: tut.ExpectPeerOnQueryStreamBuilder(t, expectedPeer, qsb, "Peers should match"),
	})

	t.Run("when providers are found, returns providers", func(t *testing.T) {
		peers := tut.RequireGenerateRetrievalPeers(t, 3)
		testResolver := testPeerResolver{peers: peers}

		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testResolver)
		testCid := testutil.GenerateCids(1)[0].Bytes()
		assert.Len(t, c.FindProviders(testCid), 3)
	})

	t.Run("when there is an error, returns empty provider list", func(t *testing.T) {
		peers := tut.RequireGenerateRetrievalPeers(t, 1)
		testResolver := testPeerResolver{peers: peers}
		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testResolver)
		badCid := []byte("badcid")
		assert.Len(t, c.FindProviders(badCid), 0)
	})

	t.Run("when there are no providers", func(t *testing.T) {
		testResolver := testPeerResolver{peers: []retrievalmarket.RetrievalPeer{}}
		c := retrievalimpl.NewClient(net, bs, &testRetrievalNode{}, &testResolver)
		testCid := testutil.GenerateCids(1)[0].Bytes()
		assert.Len(t, c.FindProviders(testCid), 0)
	})
}

type testRetrievalNode struct {
}

func (t *testRetrievalNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	return address.Address{}, nil
}

func (t *testRetrievalNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	return 0, nil
}

func (t *testRetrievalNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	return nil, nil
}

type testPeerResolver struct {
	peers []retrievalmarket.RetrievalPeer
}

var _ retrievalmarket.PeerResolver = &testPeerResolver{}

func (t testPeerResolver) GetPeers(data cid.Cid) ([]retrievalmarket.RetrievalPeer, error) {
	return t.peers, nil
}
