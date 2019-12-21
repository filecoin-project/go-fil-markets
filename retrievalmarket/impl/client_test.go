package retrievalimpl_test

import (
	"context"
	"github.com/filecoin-project/go-fil-components/retrievalmarket/network"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-components/retrievalmarket/impl"
	tut "github.com/filecoin-project/go-fil-components/retrievalmarket/network/testutil"
	"github.com/filecoin-project/go-fil-components/shared_testutil"
)

func TestClient_Query(t *testing.T) {
	ctx := context.Background()

	td := shared_testutil.NewLibp2pTestData(ctx, t)

	pcid := []byte(string("applesauce"))
	rpeer := retrievalmarket.RetrievalPeer{
		Address: address.TestAddress2,
		ID:      td.Host2.ID(),
	}

	t.Run("it works", func(t *testing.T) {
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			Host:  td.Host1,
			Peers: []peer.ID{td.Host2.ID()},
		})
		c := retrievalimpl.NewClient(
			retrievalimpl.NewClientParams{
				Host:       td.Host1,
				Blockstore: td.Bs1,
				RCNode:     &tut.TestRetrievalNode{},
				RMNet:      net,
			})

		resp, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)
	})

	t.Run("when the stream returns error, returns error", func(t *testing.T) {
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{
			Host:               td.Host1,
			Peers:              []peer.ID{td.Host2.ID()},
			QueryStreamBuilder: tut.FailNewQueryStream,
		})
		c := retrievalimpl.NewClient(
			retrievalimpl.NewClientParams{
				Host:       td.Host1,
				Blockstore: td.Bs1,
				RCNode:     &tut.TestRetrievalNode{},
				RMNet:      net,
			})

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
			Host:               td.Host2,
			Peers:              []peer.ID{td.Host1.ID()},
			QueryStreamBuilder: qsbuilder,
		})
		c := retrievalimpl.NewClient(
			retrievalimpl.NewClientParams{
				Host:       td.Host1,
				Blockstore: td.Bs1,
				RCNode:     &tut.TestRetrievalNode{},
				RMNet:      net,
			})

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
			Host:               td.Host2,
			Peers:              []peer.ID{td.Host1.ID()},
			QueryStreamBuilder: qsbuilder,
		})
		c := retrievalimpl.NewClient(
			retrievalimpl.NewClientParams{
				Host:       td.Host1,
				Blockstore: td.Bs1,
				RCNode:     &tut.TestRetrievalNode{},
				RMNet:      net,
			})

		statusCode, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		assert.EqualError(t, err, "query response failed")
		assert.Equal(t, retrievalmarket.QueryResponseUndefined, statusCode)
	})

	t.Run("use the mocknet", func(t *testing.T) {

	})
}

func TestClient_Retrieve(t *testing.T) {
}
