package shared_testutil

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/libp2p/go-libp2p-core/test"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// MakeTestSignedVoucher generates a random SignedVoucher that has all non-zero fields
func MakeTestSignedVoucher() *types.SignedVoucher {
	return &types.SignedVoucher{
		TimeLock:       rand.Uint64(),
		SecretPreimage: []byte("secret-preimage"),
		Extra:          MakeTestModVerifyParams(),
		Lane:           rand.Uint64(),
		Nonce:          rand.Uint64(),
		Amount:         MakeTestTokenAmount(),
		MinCloseHeight: rand.Uint64(),
		Merges:         []types.Merge{MakeTestMerge()},
		Signature:      MakeTestSignature(),
	}
}

// MakeTestModVerifyParams generates a random ModVerifyParams that has all non-zero fields
func MakeTestModVerifyParams() *types.ModVerifyParams {
	return &types.ModVerifyParams{
		Actor:  address.TestAddress,
		Method: rand.Uint64(),
		Data:   []byte("ModVerifyParams data"),
	}
}

// MakeTestMerge generates a random Merge that has all non-zero fields
func MakeTestMerge() types.Merge {
	return types.Merge{
		Lane:  rand.Uint64(),
		Nonce: rand.Uint64(),
	}
}

// MakeTestSignagure generates a valid yet random Signature with all non-zero fields
func MakeTestSignature() *types.Signature {
	return &types.Signature{
		Type: types.KTSecp256k1,
		Data: []byte("signature data"),
	}
}

// MakeTestTokenAmount generates a valid yet random TokenAmount with a non-zero value.
func MakeTestTokenAmount() tokenamount.TokenAmount {
	return tokenamount.TokenAmount{Int: big.NewInt(rand.Int63())}
}

// MakeTestQueryResponse generates a valid, random QueryResponse with no non-zero fields
func MakeTestQueryResponse() retrievalmarket.QueryResponse {
	return retrievalmarket.QueryResponse{
		Status:                     retrievalmarket.QueryResponseUnavailable,
		Size:                       rand.Uint64(),
		PaymentAddress:             address.TestAddress2,
		MinPricePerByte:            MakeTestTokenAmount(),
		MaxPaymentInterval:         rand.Uint64(),
		MaxPaymentIntervalIncrease: rand.Uint64(),
	}
}

// MakeTestDealProposal generates a valid, random DealProposal
func MakeTestDealProposal() retrievalmarket.DealProposal {
	cid := GenerateCids(1)[0]
	return retrievalmarket.DealProposal{
		PayloadCID: cid,
		ID:         retrievalmarket.DealID(rand.Uint64()),
		Params: retrievalmarket.Params{
			PricePerByte:            MakeTestTokenAmount(),
			PaymentInterval:         rand.Uint64(),
			PaymentIntervalIncrease: rand.Uint64(),
		},
	}
}

// MakeTestDealProposal generates a valid, random DealResponse
func MakeTestDealResponse() retrievalmarket.DealResponse {
	fakeBlk := retrievalmarket.Block{
		Prefix: []byte("prefix"),
		Data:   []byte("data"),
	}

	return retrievalmarket.DealResponse{
		Status:      retrievalmarket.DealStatusOngoing,
		ID:          retrievalmarket.DealID(rand.Uint64()),
		PaymentOwed: MakeTestTokenAmount(),
		Message:     "deal response message",
		Blocks:      []retrievalmarket.Block{fakeBlk},
	}
}

// MakeTestDealPayment generates a valid, random DealPayment
func MakeTestDealPayment() retrievalmarket.DealPayment {
	return retrievalmarket.DealPayment{
		ID:             retrievalmarket.DealID(rand.Uint64()),
		PaymentChannel: address.TestAddress,
		PaymentVoucher: MakeTestSignedVoucher(),
	}
}

// MakeTestStorageDealProposal generates a valid storage deal proposal
func MakeTestStorageDealProposal() *storagemarket.StorageDealProposal {
	return &storagemarket.StorageDealProposal{
		PieceRef:  RandomBytes(32),
		PieceSize: rand.Uint64(),

		Client:   address.TestAddress,
		Provider: address.TestAddress2,

		ProposalExpiration: rand.Uint64(),
		Duration:           rand.Uint64(),

		StoragePricePerEpoch: MakeTestTokenAmount(),
		StorageCollateral:    MakeTestTokenAmount(),

		ProposerSignature: MakeTestSignature(),
	}
}

// MakeTestStorageAsk generates a storage ask
func MakeTestStorageAsk() *types.StorageAsk {
	return &types.StorageAsk{
		Price:        MakeTestTokenAmount(),
		MinPieceSize: rand.Uint64(),
		Miner:        address.TestAddress2,
		Timestamp:    rand.Uint64(),
		Expiry:       rand.Uint64(),
		SeqNo:        rand.Uint64(),
	}
}

// MakeTestSignedStorageAsk generates a signed storage ask
func MakeTestSignedStorageAsk() *types.SignedStorageAsk {
	return &types.SignedStorageAsk{
		Ask:       MakeTestStorageAsk(),
		Signature: MakeTestSignature(),
	}
}

// MakeTestStorageNetworkProposal generates a proposal that can be sent over the
// network to a provider
func MakeTestStorageNetworkProposal() smnet.Proposal {
	return smnet.Proposal{
		DealProposal: MakeTestStorageDealProposal(),
		Piece:        &storagemarket.DataRef{Root: GenerateCids(1)[0]},
	}
}

// MakeTestStorageNetworkResponse generates a response to a proposal sent over
// the network
func MakeTestStorageNetworkResponse() smnet.Response {
	return smnet.Response{
		State:          storagemarket.StorageDealPublished,
		Proposal:       GenerateCids(1)[0],
		PublishMessage: &(GenerateCids(1)[0]),
	}
}

// MakeTestStorageNetworkSignedResponse generates a response to a proposal sent over
// the network that is signed
func MakeTestStorageNetworkSignedResponse() smnet.SignedResponse {
	return smnet.SignedResponse{
		Response:  MakeTestStorageNetworkResponse(),
		Signature: MakeTestSignature(),
	}
}

// MakeTestStorageAskRequest generates a request to get a provider's ask
func MakeTestStorageAskRequest() smnet.AskRequest {
	return smnet.AskRequest{
		Miner: address.TestAddress2,
	}
}

// MakeTestStorageAskResponse generates a response to an ask request
func MakeTestStorageAskResponse() smnet.AskResponse {
	return smnet.AskResponse{
		Ask: MakeTestSignedStorageAsk(),
	}
}

func RequireGenerateRetrievalPeers(t *testing.T, numPeers int) []retrievalmarket.RetrievalPeer {
	peers := make([]retrievalmarket.RetrievalPeer, numPeers)
	for i := range peers {
		pid, err := test.RandPeerID()
		require.NoError(t, err)
		addr, err := address.NewIDAddress(rand.Uint64())
		require.NoError(t, err)
		peers[i] = retrievalmarket.RetrievalPeer{
			Address: addr,
			ID:      pid,
		}
	}
	return peers
}
