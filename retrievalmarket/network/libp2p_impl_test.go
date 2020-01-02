package network_test

import (
	"github.com/filecoin-project/go-data-transfer/testutil"
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	"github.com/filecoin-project/go-fil-components/retrievalmarket/network"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQueryStreamSendReceiveQuery(t *testing.T) {
	// send query, read in handler
	rw := make(chan []byte)

	qs := network.NewQueryStream(requireTestPeerID(t), rw)

	cid := testutil.GenerateCids(1)[0]
	q := retrievalmarket.NewQueryV0(cid.Bytes())

	require.NoError(t, qs.WriteQuery(q))

	res, err := qs.ReadQuery()
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, cid, res.PieceCID)
}

func TestQueryStreamSendReceiveQueryResponse(t *testing.T) {
	// send response, read in handler
}

func TestQueryStreamSendReceiveMultipleSuccessful(t *testing.T) {
	// send query, read in handler, send response back, read response
}

func TestQueryStreamSendReceiveOutOfOrderFails(t *testing.T) {
	// send query, read response in handler - fails
	// send response, read query in handler - fails
}

func TestDealStreamSendReceiveDealProposal(t *testing.T) {
	// send proposal, read in handler
}

func TestDealStreamSendReceiveDealResponse(t *testing.T) {
	// send response, read in handler
}

func TestDealStreamSendReceiveDealPayment(t *testing.T) {
	// send payment, read in handler
}

func TestDealStreamSendReceiveMultipleSuccessful(t *testing.T) {
	// send proposal, read in handler, send response back, read response, send payment, read farther in hander
}

func TestQueryStreamSendReceiveMultipleOutOfOrderFails(t *testing.T) {
	// send proposal, read response in handler - fails
	// send proposal, read payment in handler - fails
	// send response, read proposal in handler - fails
	// send response, read payment in handler - fails
	// send payment, read proposal in handler - fails
	// send payment, read deal in handler - fails
}

func requireTestPeerID(t *testing.T) peer.ID {
	pid, err := test.RandPeerID()
	require.NoError(t, err)
	return pid
}
