package clientstates_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestEnsureFunds(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runEnsureFunds := makeExecutor(ctx, eventProcessor, clientstates.EnsureClientFunds, storagemarket.StorageDealEnsureClientFunds, clientDealProposal)
	addFundsCid := tut.GenerateCids(1)[0]

	t.Run("immediately succeeds", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("succeeds by sending an AddFunds message", func(t *testing.T) {
		params := nodeParams{
			AddFundsCid: addFundsCid,
		}
		runEnsureFunds(t, makeNode(params), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealClientFunding, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		n := makeNode(nodeParams{
			EnsureFundsError: errors.New("Something went wrong"),
		})
		runEnsureFunds(t, n, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "adding market funds failed: Something went wrong", deal.Message)
		})
	})
}

func TestWaitForFunding(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runEnsureFunds := makeExecutor(ctx, eventProcessor, clientstates.WaitForFunding, storagemarket.StorageDealClientFunding, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{WaitForMessageExitCode: exitcode.Ok}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{WaitForMessageExitCode: exitcode.ErrInsufficientFunds}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "adding market funds failed: AddFunds exit code: 19", deal.Message)
		})
	})
}

func TestProposeDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runProposeDeal := makeExecutor(ctx, eventProcessor, clientstates.ProposeDeal, storagemarket.StorageDealFundsEnsured, clientDealProposal)

	dealStream := func(writer tut.StorageDealProposalWriter) smnet.StorageDealStream {
		return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ProposalWriter: writer,
		})
	}

	t.Run("succeeds", func(t *testing.T) {
		runProposeDeal(t, makeNode(nodeParams{}), dealStream(tut.TrivialStorageDealProposalWriter), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealWaitingForResponse, deal.State)
		})
	})

	t.Run("write proposal fails fails", func(t *testing.T) {
		runProposeDeal(t, makeNode(nodeParams{}), dealStream(tut.FailStorageProposalWriter), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "sending proposal to storage provider failed: write proposal failed", deal.Message)
		})
	})
}

func TestVerifyResponse(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	proposalNd, err := cborutil.AsIpld(clientDealProposal)
	require.NoError(t, err)
	runVerifyResponse := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealResponse, storagemarket.StorageDealValidating, clientDealProposal)

	publishMessage := &(tut.GenerateCids(1)[0])

	t.Run("succeeds", func(t *testing.T) {
		response := storagemarket.SignedResponse{
			Response: storagemarket.ProposalResponse{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), nil, &response, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealProposalAccepted, deal.State)
			require.Equal(t, publishMessage, deal.PublishMessage)
		})
	})

	t.Run("verify response fails", func(t *testing.T) {
		response := storagemarket.SignedResponse{
			Response: storagemarket.ProposalResponse{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}
		failToVerifyNode := makeNode(nodeParams{VerifySignatureFails: true})
		runVerifyResponse(t, failToVerifyNode, nil, &response, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "unable to verify signature on deal response", deal.Message)
		})
	})

	t.Run("incorrect proposal cid", func(t *testing.T) {
		response := storagemarket.SignedResponse{
			Response: storagemarket.ProposalResponse{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       tut.GenerateCids(1)[0],
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), nil, &response, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Regexp(t, "^miner responded to a wrong proposal:", deal.Message)
		})
	})

	t.Run("deal rejected", func(t *testing.T) {
		response := storagemarket.SignedResponse{
			Response: storagemarket.ProposalResponse{
				State:          storagemarket.StorageDealProposalRejected,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
				Message:        "because reasons",
			},
			Signature: tut.MakeTestSignature(),
		}
		expErr := fmt.Sprintf("deal failed: (State=%d) because reasons", storagemarket.StorageDealProposalRejected)
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), nil, &response, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, deal.Message, expErr)
		})
	})

}

func TestValidateDealPublished(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runValidateDealPublished := makeExecutor(ctx, eventProcessor, clientstates.ValidateDealPublished, storagemarket.StorageDealProposalAccepted, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runValidateDealPublished(t, makeNode(nodeParams{ValidatePublishedDealID: abi.DealID(5)}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealSealing, deal.State)
			require.Equal(t, abi.DealID(5), deal.DealID)
		})
	})

	t.Run("fails", func(t *testing.T) {
		n := makeNode(nodeParams{
			ValidatePublishedDealID: abi.DealID(5),
			ValidatePublishedError:  errors.New("Something went wrong"),
		})
		runValidateDealPublished(t, n, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error validating deal published: Something went wrong", deal.Message)
		})
	})
}

func TestVerifyDealActivated(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealActivated, storagemarket.StorageDealSealing, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealActive, deal.State)
		})
	})

	t.Run("fails synchronously", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{DealCommittedSyncError: errors.New("Something went wrong")}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error in deal activation: Something went wrong", deal.Message)
		})
	})

	t.Run("fails asynchronously", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{DealCommittedAsyncError: errors.New("Something went wrong later")}), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error in deal activation: Something went wrong later", deal.Message)
		})
	})
}

func TestFailDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runFailDeal := makeExecutor(ctx, eventProcessor, clientstates.FailDeal, storagemarket.StorageDealFailing, clientDealProposal)

	t.Run("success", func(t *testing.T) {
		runFailDeal(t, nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
		})
	})

}

type executor func(t *testing.T,
	node storagemarket.StorageClientNode,
	dealStream smnet.StorageDealStream,
	response *storagemarket.SignedResponse,
	dealInspector func(deal storagemarket.ClientDeal))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc clientstates.ClientStateEntryFunc,
	initialState storagemarket.StorageDealStatus,
	clientDealProposal *market.ClientDealProposal) executor {
	return func(t *testing.T,
		node storagemarket.StorageClientNode,
		dealStream smnet.StorageDealStream,
		response *storagemarket.SignedResponse,
		dealInspector func(deal storagemarket.ClientDeal)) {
		dealState, err := tut.MakeTestClientDeal(initialState, clientDealProposal)
		dealState.AddFundsCid = &tut.GenerateCids(1)[0]
		dealState.LastResponse = response
		require.NoError(t, err)
		environment := &fakeEnvironment{node, dealStream}
		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(*dealState)
	}
}

type nodeParams struct {
	AddFundsCid             cid.Cid
	EnsureFundsError        error
	VerifySignatureFails    bool
	GetBalanceError         error
	GetChainHeadError       error
	WaitForMessageBlocks    bool
	WaitForMessageError     error
	WaitForMessageExitCode  exitcode.ExitCode
	WaitForMessageRetBytes  []byte
	ClientAddr              address.Address
	ValidationError         error
	ValidatePublishedDealID abi.DealID
	ValidatePublishedError  error
	DealCommittedSyncError  error
	DealCommittedAsyncError error
}

func makeNode(params nodeParams) storagemarket.StorageClientNode {
	var out testnodes.FakeClientNode
	out.SMState = testnodes.NewStorageMarketState()
	out.AddFundsCid = params.AddFundsCid
	out.EnsureFundsError = params.EnsureFundsError
	out.VerifySignatureFails = params.VerifySignatureFails
	out.GetBalanceError = params.GetBalanceError
	out.GetChainHeadError = params.GetChainHeadError
	out.WaitForMessageBlocks = params.WaitForMessageBlocks
	out.WaitForMessageError = params.WaitForMessageError
	out.WaitForMessageExitCode = params.WaitForMessageExitCode
	out.WaitForMessageRetBytes = params.WaitForMessageRetBytes
	out.ClientAddr = params.ClientAddr
	out.ValidationError = params.ValidationError
	out.ValidatePublishedDealID = params.ValidatePublishedDealID
	out.ValidatePublishedError = params.ValidatePublishedError
	out.DealCommittedSyncError = params.DealCommittedSyncError
	out.DealCommittedAsyncError = params.DealCommittedAsyncError
	return &out
}

type fakeEnvironment struct {
	node       storagemarket.StorageClientNode
	dealStream smnet.StorageDealStream
}

func (fe *fakeEnvironment) Node() storagemarket.StorageClientNode {
	return fe.node
}

func (fe *fakeEnvironment) WriteDealProposal(p peer.ID, proposal storagemarket.ProposalRequest) error {
	return fe.dealStream.WriteDealProposal(proposal)
}
