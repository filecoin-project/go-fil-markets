package retrievalimpl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
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
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storedcounter"
)

func TestClient_Query(t *testing.T) {
	ctx := context.Background()

	ds := dss.MutexWrap(datastore.NewMapDatastore())
	storedCounter := storedcounter.New(ds, datastore.NewKey("nextDealID"))
	bs := bstore.NewBlockstore(ds)

	pcid := tut.GenerateCids(1)[0]
	expectedPeer := peer.ID("somevalue")
	rpeer := retrievalmarket.RetrievalPeer{
		Address: address.TestAddress2,
		ID:      expectedPeer,
	}

	expectedQuery := retrievalmarket.Query{
		PayloadCID: pcid,
	}

	expectedQueryResponse := retrievalmarket.QueryResponse{
		Status:                     retrievalmarket.QueryResponseAvailable,
		Size:                       1234,
		PaymentAddress:             address.TestAddress,
		MinPricePerByte:            abi.NewTokenAmount(5678),
		MaxPaymentInterval:         4321,
		MaxPaymentIntervalIncrease: 0,
	}

	testCases := []struct {
		name    string
		expErr  string
		expResp retrievalmarket.QueryResponse
		qBldr   tut.QueryStreamBuilder
	}{
		{name: "it works",
			expResp: expectedQueryResponse,
			qBldr: func(p peer.ID) (rmnet.RetrievalQueryStream, error) {
				return tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
					Writer:     tut.ExpectQueryWriter(t, expectedQuery, "queries should match"),
					RespReader: tut.StubbedQueryResponseReader(expectedQueryResponse),
				}), nil
			},
		},
		{name: "when the stream returns error, returns error",
			expErr:  "new query stream failed",
			expResp: retrievalmarket.QueryResponseUndefined,
			qBldr:   tut.FailNewQueryStream,
		},
		{name: "when ReadQueryResponse fails, returns error",
			expErr:  "query response failed",
			expResp: retrievalmarket.QueryResponseUndefined,
			qBldr: func(p peer.ID) (network.RetrievalQueryStream, error) {
				newStream := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
					PeerID:     p,
					RespReader: tut.FailResponseReader,
				})
				return newStream, nil
			},
		},
		{name: "when WriteQuery fails, returns error",
			expErr:  "write query failed",
			expResp: retrievalmarket.QueryResponseUndefined,
			qBldr: func(p peer.ID) (network.RetrievalQueryStream, error) {
				newStream := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
					PeerID: p,
					Writer: tut.FailQueryWriter,
				})
				return newStream, nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
				QueryStreamBuilder: tc.qBldr,
			})
			c, err := retrievalimpl.NewClient(
				net,
				bs,
				testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{}),
				&testPeerResolver{},
				ds,
				storedCounter)
			require.NoError(t, err)

			actualResp, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
			if tc.expErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expErr)
			}
			assert.Equal(t, tc.expResp, actualResp)
		})
	}
}

func TestClient_FindProviders(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	storedCounter := storedcounter.New(ds, datastore.NewKey("nextDealID"))
	bs := bstore.NewBlockstore(ds)
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

		c, err := retrievalimpl.NewClient(net, bs, &testnodes.TestRetrievalClientNode{}, &testResolver, ds, storedCounter)
		require.NoError(t, err)

		testCid := tut.GenerateCids(1)[0]
		assert.Len(t, c.FindProviders(testCid), 3)
	})

	t.Run("when there is an error, returns empty provider list", func(t *testing.T) {
		testResolver := testPeerResolver{peers: []retrievalmarket.RetrievalPeer{}, resolverError: errors.New("boom")}
		c, err := retrievalimpl.NewClient(net, bs, &testnodes.TestRetrievalClientNode{}, &testResolver, ds, storedCounter)
		require.NoError(t, err)

		badCid := tut.GenerateCids(1)[0]
		assert.Len(t, c.FindProviders(badCid), 0)
	})

	t.Run("when there are no providers", func(t *testing.T) {
		testResolver := testPeerResolver{peers: []retrievalmarket.RetrievalPeer{}}
		c, err := retrievalimpl.NewClient(net, bs, &testnodes.TestRetrievalClientNode{}, &testResolver, ds, storedCounter)
		require.NoError(t, err)

		testCid := tut.GenerateCids(1)[0]
		assert.Len(t, c.FindProviders(testCid), 0)
	})
}

type testPeerResolver struct {
	peers         []retrievalmarket.RetrievalPeer
	resolverError error
}

var _ retrievalmarket.PeerResolver = &testPeerResolver{}

func (tpr testPeerResolver) GetPeers(cid.Cid) ([]retrievalmarket.RetrievalPeer, error) {
	return tpr.peers, tpr.resolverError
}
