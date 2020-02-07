package network

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

//go:generate cbor-gen-for AskRequest AskResponse Proposal Response SignedResponse

// Proposal is the data sent over the network from client to provider when proposing
// a deal
type Proposal struct {
	DealProposal *storagemarket.StorageDealProposal

	Piece cid.Cid // Used for retrieving from the client
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

	Signature *types.Signature
}

var SignedResponseUndefined = SignedResponse{}

// Verify verifies that a proposal was signed by the given provider
func (r *SignedResponse) Verify(addr address.Address, verifier storagemarket.SignatureVerifier) error {
	b, err := cborutil.Dump(&r.Response)
	if err != nil {
		return err
	}

	return verifier(addr, b)
}

// AskRequest is a request for current ask parameters for a given miner
type AskRequest struct {
	Miner address.Address
}

var AskRequestUndefined = AskRequest{}

// AskResponse is the response sent over the network in response
// to an ask request
type AskResponse struct {
	Ask *types.SignedStorageAsk
}

var AskResponseUndefined = AskResponse{}
