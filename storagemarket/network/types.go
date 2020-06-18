package network

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

//go:generate cbor-gen-for AskRequest AskResponse Proposal Response SignedResponse QueryRequest QueryResponse

// Proposal is the data sent over the network from client to provider when proposing
// a deal
type Proposal struct {
	DealProposal *market.ClientDealProposal

	Piece *storagemarket.DataRef
}

var ProposalUndefined = Proposal{}

// Response is a response to a proposal sent over the network
type Response struct {
	State storagemarket.StorageDealStatus

	// DealProposalRejected
	Message  string
	Proposal cid.Cid

	// StorageDealProposalAccepted
	PublishMessage *cid.Cid
}

// SignedResponse is a response that is signed
type SignedResponse struct {
	Response Response

	Signature *crypto.Signature
}

var SignedResponseUndefined = SignedResponse{}

// AskRequest is a request for current ask parameters for a given miner
type AskRequest struct {
	Miner address.Address
}

var AskRequestUndefined = AskRequest{}

// AskResponse is the response sent over the network in response
// to an ask request
type AskResponse struct {
	Ask *storagemarket.SignedStorageAsk
}

var AskResponseUndefined = AskResponse{}

// QueryRequest is the data sent over the network from client to provider when querying a deal
type QueryRequest struct {
	Proposal cid.Cid
}

var QueryRequestUndefined = QueryRequest{}

// QueryResponse is a response to a proposal sent over the network
type QueryResponse struct {
	DealState *storagemarket.ProviderDealState
}

var QueryResponseUndefined = QueryResponse{}
