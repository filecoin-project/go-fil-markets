package clientstates_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
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
		runEnsureFunds(t, nodeParams{}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("succeeds by sending an AddFunds message", func(t *testing.T) {
		nodeParams := nodeParams{
			AddFundsCid: addFundsCid,
		}
		runEnsureFunds(t, nodeParams, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealClientFunding, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		nodeParams := nodeParams{
			EnsureFundsError: errors.New("Something went wrong"),
		}
		runEnsureFunds(t, nodeParams, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
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
		runEnsureFunds(t, nodeParams{WaitForMessageExitCode: exitcode.Ok}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFundsEnsured, deal.State)
		})
	})

	t.Run("EnsureClientFunds fails", func(t *testing.T) {
		runEnsureFunds(t, nodeParams{WaitForMessageExitCode: exitcode.ErrInsufficientFunds}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
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

	dealStream := func(writer tut.StorageDealProposalWriter) *tut.TestStorageDealStream {
		return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ProposalWriter: writer,
		})
	}

	t.Run("succeeds", func(t *testing.T) {
		ds := dealStream(tut.TrivialStorageDealProposalWriter)
		runProposeDeal(t, nodeParams{}, envParams{dealStream: ds}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealWaitingForDataRequest, deal.State)
			ds.AssertConnectionTagged(t, deal.ProposalCid.String())
		})
	})

	t.Run("write proposal fails fails", func(t *testing.T) {
		runProposeDeal(t, nodeParams{}, envParams{dealStream: dealStream(tut.FailStorageProposalWriter)}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "sending proposal to storage provider failed: write proposal failed", deal.Message)
		})
	})
}

func TestWaitingForDataRequest(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	require.NoError(t, err)
	runWaitingForDataRequest := makeExecutor(ctx, eventProcessor, clientstates.WaitingForDataRequest, storagemarket.StorageDealWaitingForDataRequest, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealWaitingForData,
		})

		runWaitingForDataRequest(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			require.Len(t, env.startDataTransferCalls, 1)
			require.Equal(t, env.startDataTransferCalls[0].to, deal.Miner)
			require.Equal(t, env.startDataTransferCalls[0].baseCid, deal.DataRef.Root)

			tut.AssertDealState(t, storagemarket.StorageDealTransferring, deal.State)
		})
	})

	t.Run("response contains unexpected state", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealProposalNotFound,
		})

		runWaitingForDataRequest(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "unexpected deal status while waiting for data request: 1", deal.Message)
		})
	})

	t.Run("read response fails", func(t *testing.T) {
		stream := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: tut.FailStorageResponseReader,
		})

		runWaitingForDataRequest(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error reading Response message: read response failed", deal.Message)
		})
	})

	t.Run("fails starting the data transfer request", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealWaitingForData,
		})
		envParams := envParams{
			dealStream:             stream,
			startDataTransferError: errors.New("failed"),
		}

		runWaitingForDataRequest(t, nodeParams{}, envParams, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "failed to initiate data transfer: failed to open push data channel: failed", deal.Message)
		})
	})

	t.Run("waits for another response with manual transfers", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealWaitingForData,
		})
		envParams := envParams{
			dealStream:     stream,
			manualTransfer: true,
		}

		runWaitingForDataRequest(t, nodeParams{}, envParams, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealValidating, deal.State)
		})
	})
}

func TestVerifyResponse(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	require.NoError(t, err)
	runVerifyResponse := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealResponse, storagemarket.StorageDealValidating, clientDealProposal)

	t.Run("succeeds", func(t *testing.T) {
		publishMessage := &(tut.GenerateCids(1)[0])
		stream := testResponseStream(t, responseParams{
			proposal:       clientDealProposal,
			state:          storagemarket.StorageDealProposalAccepted,
			publishMessage: publishMessage,
		})

		runVerifyResponse(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealProposalAccepted, deal.State)
			require.Equal(t, publishMessage, deal.PublishMessage)
		})
	})

	t.Run("read response fails", func(t *testing.T) {
		stream := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: tut.FailStorageResponseReader,
		})

		runVerifyResponse(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error reading Response message: read response failed", deal.Message)
		})
	})

	t.Run("verify response fails", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealProposalAccepted,
		})

		runVerifyResponse(t, nodeParams{VerifySignatureFails: true}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, "unable to verify signature on deal response", deal.Message)
		})
	})

	t.Run("incorrect proposal cid", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal:    clientDealProposal,
			state:       storagemarket.StorageDealProposalAccepted,
			proposalCid: tut.GenerateCids(1)[0],
		})

		runVerifyResponse(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			require.Regexp(t, "^miner responded to a wrong proposal:", deal.Message)
		})
	})

	t.Run("deal rejected", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealProposalRejected,
			message:  "because reasons",
		})

		expErr := fmt.Sprintf("deal failed: (State=%d) because reasons", storagemarket.StorageDealProposalRejected)
		runVerifyResponse(t, nodeParams{}, envParams{dealStream: stream}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			require.Equal(t, deal.Message, expErr)
		})
	})

	t.Run("deal stream close errors", func(t *testing.T) {
		stream := testResponseStream(t, responseParams{
			proposal: clientDealProposal,
			state:    storagemarket.StorageDealProposalAccepted,
		})
		closeStreamErr := errors.New("something went wrong")

		runVerifyResponse(t, nodeParams{}, envParams{dealStream: stream, closeStreamErr: closeStreamErr}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error attempting to close stream: something went wrong", deal.Message)
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
		runValidateDealPublished(t, nodeParams{ValidatePublishedDealID: abi.DealID(5)}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealSealing, deal.State)
			require.Equal(t, abi.DealID(5), deal.DealID)
		})
	})

	t.Run("fails", func(t *testing.T) {
		nodeParams := nodeParams{
			ValidatePublishedDealID: abi.DealID(5),
			ValidatePublishedError:  errors.New("Something went wrong"),
		}
		runValidateDealPublished(t, nodeParams, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
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
		runVerifyDealActivated(t, nodeParams{}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealActive, deal.State)
		})
	})

	t.Run("fails synchronously", func(t *testing.T) {
		runVerifyDealActivated(t, nodeParams{DealCommittedSyncError: errors.New("Something went wrong")}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error in deal activation: Something went wrong", deal.Message)
		})
	})

	t.Run("fails asynchronously", func(t *testing.T) {
		runVerifyDealActivated(t, nodeParams{DealCommittedAsyncError: errors.New("Something went wrong later")}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
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

	t.Run("able to close stream", func(t *testing.T) {
		runFailDeal(t, nodeParams{}, envParams{}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
		})
	})

	t.Run("unable to close stream", func(t *testing.T) {
		runFailDeal(t, nodeParams{}, envParams{closeStreamErr: errors.New("unable to close")}, func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
			tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			require.Equal(t, "error attempting to close stream: unable to close", deal.Message)
		})
	})
}

type envParams struct {
	dealStream             smnet.StorageDealStream
	closeStreamErr         error
	startDataTransferError error
	manualTransfer         bool
}

type executor func(t *testing.T,
	nodeParams nodeParams,
	envParams envParams,
	dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc clientstates.ClientStateEntryFunc,
	initialState storagemarket.StorageDealStatus,
	clientDealProposal *market.ClientDealProposal) executor {
	return func(t *testing.T,
		nodeParams nodeParams,
		envParams envParams,
		dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment)) {
		node := makeNode(nodeParams)
		dealState, err := tut.MakeTestClientDeal(initialState, clientDealProposal, envParams.manualTransfer)
		require.NoError(t, err)
		dealState.AddFundsCid = &tut.GenerateCids(1)[0]

		environment := &fakeEnvironment{
			node:                   node,
			dealStream:             envParams.dealStream,
			closeStreamErr:         envParams.closeStreamErr,
			startDataTransferError: envParams.startDataTransferError,
		}
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
	node                   storagemarket.StorageClientNode
	dealStream             smnet.StorageDealStream
	closeStreamErr         error
	startDataTransferError error
	startDataTransferCalls []dataTransferParams
}

type dataTransferParams struct {
	to       peer.ID
	voucher  datatransfer.Voucher
	baseCid  cid.Cid
	selector ipld.Node
}

func (fe *fakeEnvironment) StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	fe.startDataTransferCalls = append(fe.startDataTransferCalls, dataTransferParams{
		to:       to,
		voucher:  voucher,
		baseCid:  baseCid,
		selector: selector,
	})
	return fe.startDataTransferError
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
	return fe.closeStreamErr
}

var _ clientstates.ClientDealEnvironment = &fakeEnvironment{}

type responseParams struct {
	proposal       *market.ClientDealProposal
	state          storagemarket.StorageDealStatus
	message        string
	publishMessage *cid.Cid
	proposalCid    cid.Cid
}

func testResponseStream(t *testing.T, params responseParams) smnet.StorageDealStream {
	response := smnet.Response{
		State:          params.state,
		Proposal:       params.proposalCid,
		Message:        params.message,
		PublishMessage: params.publishMessage,
	}

	if response.Proposal == cid.Undef {
		proposalNd, err := cborutil.AsIpld(params.proposal)
		require.NoError(t, err)
		response.Proposal = proposalNd.Cid()
	}

	reader := tut.StubbedStorageResponseReader(smnet.SignedResponse{
		Response:  response,
		Signature: tut.MakeTestSignature(),
	})

	return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
		ResponseReader: reader,
	})
}
