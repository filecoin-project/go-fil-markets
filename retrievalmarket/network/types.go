package network

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

//go:generate cbor-gen-for --map-encoding AskRequest AskResponse

// AskRequest is a request for current retrieval ask parameters
type AskRequest struct {
	Miner address.Address
}

// AskRequestUndefined represents an empty AskRequest message
var AskRequestUndefined = AskRequest{}

// AskResponse is the response sent over the network in response
// to an ask request
type AskResponse struct {
	Ask *retrievalmarket.Ask
}

// AskResponseUndefined represents an empty AskResponse message
var AskResponseUndefined = AskResponse{}
