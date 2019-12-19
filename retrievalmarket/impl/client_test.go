package retrievalimpl_test

import (
	"context"
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-components/retrievalmarket/impl"
	tut "github.com/filecoin-project/go-fil-components/retrievalmarket/network/testutil"
	"github.com/filecoin-project/go-fil-components/shared/address"
	"github.com/filecoin-project/go-fil-components/shared_testutil"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
		net := tut.NewTestRetrievalMarketNetwork(td.Host1, []peer.ID{td.Host2.ID()})
		c := retrievalimpl.NewClient(
			retrievalimpl.NewClientParams{
				Host:       td.Host1,
				Blockstore: td.Bs1,
				RCNode:     tut.TestRetrievalNode{},
				RMNet:      net,
			})

		resp, err := c.Query(ctx, rpeer, pcid, retrievalmarket.QueryParams{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})
}
