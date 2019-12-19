package retrievalimpl_test

import (
	"context"
	retrievalimpl "github.com/filecoin-project/go-fil-components/retrievalmarket/impl"
	tut "github.com/filecoin-project/go-fil-components/retrievalmarket/network/testutil"
	"github.com/libp2p/go-libp2p-core/peer"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
)

func TestClient_Query(t *testing.T) {
	ctx := context.Background()


	mnet := mocknet.New(ctx)
	h1, err := mnet.GenPeer()
	require.NoError(t, err)
	h2, err := mnet.GenPeer()
	require.NoError(t, err)

	bs :=

	t.Run("it works", func(t *testing.T) {
		net := tut.NewTestRetrievalMarketNetwork(h1, []peer.ID{h2.ID()})

		c := retrievalimpl.NewClient(h1, bs, n)
	})
}
