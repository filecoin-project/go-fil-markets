package network_test

import (
	"testing"
)

func TestQueryStreamSendReceiveQuery(t *testing.T) {
	// send query, read in handler

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
