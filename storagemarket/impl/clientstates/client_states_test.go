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
	runEnsureFunds := makeExecutor(ctx, eventProcessor, clientstates.EnsureClientFunds, clientDealParams{state: storagemarket.StorageDealEnsureClientFunds}, clientDealProposal)
	addFundsCid := tut.GenerateCids(1)[0]

	t.Run("immediately succeeds", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("succeeds by sending an AddFunds message", func(t *testing.T) {
		params := nodeParams{
			AddFundsCid: addFundsCid,
		}
		runEnsureFunds(t, makeNode(params), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealClientFunding, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		n := makeNode(nodeParams{
			EnsureFundsError: errors.New("Something went wrong"),
		})
		runEnsureFunds(t, n, nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
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
	runEnsureFunds := makeExecutor(ctx, eventProcessor, clientstates.WaitForFunding, clientDealParams{state: storagemarket.StorageDealClientFunding}, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{WaitForMessageExitCode: exitcode.Ok}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		runEnsureFunds(t, makeNode(nodeParams{WaitForMessageExitCode: exitcode.ErrInsufficientFunds}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
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
	runProposeDeal := makeExecutor(ctx, eventProcessor, clientstates.ProposeDeal, clientDealParams{state: storagemarket.StorageDealFundsEnsured}, clientDealProposal)

	dealStream := func(writer tut.StorageDealProposalWriter) *tut.TestStorageDealStream {
		return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ProposalWriter: writer,
		})
	}

	t.Run("succeeds", func(t *testing.T) {
		ds := dealStream(tut.TrivialStorageDealProposalWriter)
		runProposeDeal(t, makeNode(nodeParams{}), ds, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealValidating, deal.State)
			ds.AssertConnectionTagged(t, deal.ProposalCid.String())
		})
	})

	t.Run("write proposal fails fails", func(t *testing.T) {
		runProposeDeal(t, makeNode(nodeParams{}), dealStream(tut.FailStorageProposalWriter), nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
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
	runVerifyResponse := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealResponse, clientDealParams{state: storagemarket.StorageDealValidating}, clientDealProposal)

	publishMessage := &(tut.GenerateCids(1)[0])

	dealStream := func(reader tut.StorageDealResponseReader) smnet.StorageDealStream {
		return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: reader,
		})
	}

	t.Run("succeeds", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}))
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), stream, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealProposalAccepted, deal.State)
			require.Equal(t, publishMessage, deal.PublishMessage)
		})
	})

	t.Run("read response fails", func(t *testing.T) {
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), dealStream(tut.FailStorageResponseReader), nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error reading Response message: read response failed", deal.Message)
		})
	})

	t.Run("verify response fails", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}))
		failToVerifyNode := makeNode(nodeParams{VerifySignatureFails: true})
		runVerifyResponse(t, failToVerifyNode, stream, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "unable to verify signature on deal response", deal.Message)
		})
	})

	t.Run("incorrect proposal cid", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       tut.GenerateCids(1)[0],
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}))
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), stream, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Regexp(t, "^miner responded to a wrong proposal:", deal.Message)
		})
	})

	t.Run("deal rejected", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalRejected,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
				Message:        "because reasons",
			},
			Signature: tut.MakeTestSignature(),
		}))
		expErr := fmt.Sprintf("deal failed: (State=%d) because reasons", storagemarket.StorageDealProposalRejected)
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), stream, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, deal.Message, expErr)
		})
	})

	t.Run("deal stream close errors", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalAccepted,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}))
		closeStreamErr := errors.New("something went wrong")
		runVerifyResponse(t, makeNode(nodeParams{VerifySignatureFails: false}), stream, closeStreamErr, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error attempting to close stream: something went wrong", deal.Message)
		})
	})

}

func TestValidateDealPublished(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runValidateDealPublished := makeExecutor(ctx, eventProcessor, clientstates.ValidateDealPublished, clientDealParams{state: storagemarket.StorageDealProposalAccepted}, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runValidateDealPublished(t, makeNode(nodeParams{ValidatePublishedDealID: abi.DealID(5)}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealSealing, deal.State)
			require.Equal(t, abi.DealID(5), deal.DealID)
		})
	})

	t.Run("fails", func(t *testing.T) {
		n := makeNode(nodeParams{
			ValidatePublishedDealID: abi.DealID(5),
			ValidatePublishedError:  errors.New("Something went wrong"),
		})
		runValidateDealPublished(t, n, nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
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
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealActivated, clientDealParams{state: storagemarket.StorageDealSealing}, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealActive, deal.State)
		})
	})

	t.Run("fails synchronously", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{DealCommittedSyncError: errors.New("Something went wrong")}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Equal(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error in deal activation: Something went wrong", deal.Message)
		})
	})

	t.Run("fails asynchronously", func(t *testing.T) {
		runVerifyDealActivated(t, makeNode(nodeParams{DealCommittedAsyncError: errors.New("Something went wrong later")}), nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
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

	t.Run("with an open connection", func(t *testing.T) {
		stateParams := clientDealParams{
			state:            storagemarket.StorageDealFailing,
			connectionClosed: false,
		}
		runFailDeal := makeExecutor(ctx, eventProcessor, clientstates.FailDeal, stateParams, clientDealProposal)

		t.Run("able to close stream", func(t *testing.T) {
			runFailDeal(t, nil, nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				require.Equal(t, storagemarket.StorageDealError, deal.State)
			})
		})

		t.Run("unable to close stream", func(t *testing.T) {
			runFailDeal(t, nil, nil, errors.New("unable to close"), func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				require.Equal(t, storagemarket.StorageDealError, deal.State)
				require.Equal(t, "error attempting to close stream: unable to close", deal.Message)
			})
		})
	})

	t.Run("with a closed connection", func(t *testing.T) {
		stateParams := clientDealParams{
			state:            storagemarket.StorageDealFailing,
			connectionClosed: true,
		}
		runFailDeal := makeExecutor(ctx, eventProcessor, clientstates.FailDeal, stateParams, clientDealProposal)

		t.Run("doesn't attempt to close stream if not open", func(t *testing.T) {
			runFailDeal(t, nil, nil, nil, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				require.Len(t, env.closeStreamCalls, 0)
				require.Equal(t, storagemarket.StorageDealError, deal.State)
			})
		})
	})
}

type executor func(t *testing.T,
	node storagemarket.StorageClientNode,
	dealStream smnet.StorageDealStream,
	closeStreamErr error,
	dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment))

type clientDealParams struct {
	state            storagemarket.StorageDealStatus
	connectionClosed bool
	addFundsCid      *cid.Cid
}

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc clientstates.ClientStateEntryFunc,
	dealParams clientDealParams,
	clientDealProposal *market.ClientDealProposal) executor {
	return func(t *testing.T,
		node storagemarket.StorageClientNode,
		dealStream smnet.StorageDealStream,
		closeStreamErr error,
		dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment)) {

		dealState, err := tut.MakeTestClientDeal(dealParams.state, clientDealProposal)
		require.NoError(t, err)

		dealState.ConnectionClosed = dealParams.connectionClosed
		dealState.AddFundsCid = &tut.GenerateCids(1)[0]

		if dealParams.addFundsCid != nil {
			dealState.AddFundsCid = dealParams.addFundsCid
		}

		require.NoError(t, err)
		environment := &fakeEnvironment{node, dealStream, closeStreamErr, nil}
		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(*dealState, environment)
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
	node             storagemarket.StorageClientNode
	dealStream       smnet.StorageDealStream
	closeStreamErr   error
	closeStreamCalls []cid.Cid
}

func (fe *fakeEnvironment) Node() storagemarket.StorageClientNode {
	return fe.node
}

func (fe *fakeEnvironment) WriteDealProposal(p peer.ID, proposalCid cid.Cid, proposal smnet.Proposal) error {
	return fe.dealStream.WriteDealProposal(proposal)
}

func (fe *fakeEnvironment) ReadDealResponse(proposalCid cid.Cid) (smnet.SignedResponse, error) {
	return fe.dealStream.ReadDealResponse()
}

func (fe *fakeEnvironment) TagConnection(proposalCid cid.Cid) error {
	fe.dealStream.TagProtectedConnection(proposalCid.String())
	return nil
}

func (fe *fakeEnvironment) CloseStream(proposalCid cid.Cid) error {
	fe.closeStreamCalls = append(fe.closeStreamCalls, proposalCid)
	return fe.closeStreamErr
}
