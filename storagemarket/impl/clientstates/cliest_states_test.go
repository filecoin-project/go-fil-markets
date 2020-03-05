package clientstates_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	cborutil "github.com/filecoin-project/go-cbor-util"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

func TestEnsureFunds(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runEnsureFunds := makeExecutor(ctx, eventProcessor, clientstates.EnsureFunds, storagemarket.StorageDealUnknown, clientDealProposal)

	node := func(ensureFundsErr error) storagemarket.StorageClientNode {
		return &testnodes.FakeClientNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState:          testnodes.NewStorageMarketState(),
				EnsureFundsError: ensureFundsErr,
			},
		}
	}
	t.Run("EnsureFunds succeeds", func(t *testing.T) {
		runEnsureFunds(t, node(nil), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFundsEnsured)
		})
	})

	t.Run("EnsureFunds fails", func(t *testing.T) {
		runEnsureFunds(t, node(errors.New("Something went wrong")), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "adding market funds failed: Something went wrong")
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

	node := func() storagemarket.StorageClientNode {
		return &testnodes.FakeClientNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
		}
	}

	t.Run("succeeds", func(t *testing.T) {
		runProposeDeal(t, node(), nil, dealStream(tut.TrivialStorageDealProposalWriter), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealValidating)
		})
	})

	t.Run("deal stream lookup fails", func(t *testing.T) {
		runProposeDeal(t, node(), errors.New("deal stream not found"), nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "miner connection error: deal stream not found")
		})
	})

	t.Run("write proposal fails fails", func(t *testing.T) {
		runProposeDeal(t, node(), nil, dealStream(tut.FailStorageProposalWriter), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealError)
			require.Equal(t, deal.Message, "sending proposal to storage provider failed: write proposal failed")
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

	dealStream := func(reader tut.StorageDealResponseReader) smnet.StorageDealStream {
		return tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: reader,
		})
	}

	node := func(verifySignatureFails bool) storagemarket.StorageClientNode {
		return &testnodes.FakeClientNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState:              testnodes.NewStorageMarketState(),
				VerifySignatureFails: verifySignatureFails,
			},
		}
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
		runVerifyResponse(t, node(false), nil, stream, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealProposalAccepted)
			require.Equal(t, deal.PublishMessage, publishMessage)
		})
	})

	t.Run("deal stream lookup fails", func(t *testing.T) {
		dealStreamErr := errors.New("deal stream not found")
		runVerifyResponse(t, node(false), dealStreamErr, dealStream(nil), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "miner connection error: deal stream not found")
		})
	})

	t.Run("read response fails", func(t *testing.T) {
		runVerifyResponse(t, node(false), nil, dealStream(tut.FailStorageResponseReader), nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealError)
			require.Equal(t, deal.Message, "error reading Response message: read response failed")
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
		failToVerifyNode := node(true)
		runVerifyResponse(t, failToVerifyNode, nil, stream, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "unable to verify signature on deal response")
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
		runVerifyResponse(t, node(false), nil, stream, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Regexp(t, "^miner responded to a wrong proposal:", deal.Message)
		})
	})

	t.Run("incorrect proposal cid", func(t *testing.T) {
		stream := dealStream(tut.StubbedStorageResponseReader(smnet.SignedResponse{
			Response: smnet.Response{
				State:          storagemarket.StorageDealProposalRejected,
				Proposal:       proposalNd.Cid(),
				PublishMessage: publishMessage,
			},
			Signature: tut.MakeTestSignature(),
		}))
		runVerifyResponse(t, node(false), nil, stream, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, fmt.Sprintf("deal wasn't accepted (State=%d)", storagemarket.StorageDealProposalRejected))
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
		runVerifyResponse(t, node(false), nil, stream, closeStreamErr, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealError)
			require.Equal(t, deal.Message, "error attempting to close stream: something went wrong")
		})
	})

}

func TestValidateDealPublished(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runValidateDealPublished := makeExecutor(ctx, eventProcessor, clientstates.ValidateDealPublished, storagemarket.StorageDealProposalAccepted, clientDealProposal)

	node := func(dealID abi.DealID, validatePublishedErr error) storagemarket.StorageClientNode {
		return &testnodes.FakeClientNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
			ValidatePublishedDealID: dealID,
			ValidatePublishedError:  validatePublishedErr,
		}
	}

	t.Run("succeeds", func(t *testing.T) {
		runValidateDealPublished(t, node(abi.DealID(5), nil), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealPublished)
			require.Equal(t, deal.DealID, abi.DealID(5))
		})
	})

	t.Run("fails", func(t *testing.T) {
		runValidateDealPublished(t, node(abi.DealID(5), errors.New("Something went wrong")), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "error validating deal published: Something went wrong")
		})
	})
}

func TestVerifyDealActivated(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	require.NoError(t, err)
	clientDealProposal := tut.MakeTestClientDealProposal()
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, clientstates.VerifyDealActivated, storagemarket.StorageDealPublished, clientDealProposal)

	node := func(syncError error, asyncError error) storagemarket.StorageClientNode {
		return &testnodes.FakeClientNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
			DealCommittedSyncError:  syncError,
			DealCommittedAsyncError: asyncError,
		}
	}

	t.Run("succeeds", func(t *testing.T) {
		runVerifyDealActivated(t, node(nil, nil), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealActive)
		})
	})

	t.Run("fails synchronously", func(t *testing.T) {
		runVerifyDealActivated(t, node(errors.New("Something went wrong"), nil), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "error in deal activation: Something went wrong")
		})
	})

	t.Run("fails asynchronously", func(t *testing.T) {
		runVerifyDealActivated(t, node(nil, errors.New("Something went wrong later")), nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealFailing)
			require.Equal(t, deal.Message, "error in deal activation: Something went wrong later")
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
		runFailDeal(t, nil, nil, nil, nil, func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealError)
		})
	})

	t.Run("unable to close stream", func(t *testing.T) {
		runFailDeal(t, nil, nil, nil, errors.New("unable to close"), func(deal storagemarket.ClientDeal) {
			require.Equal(t, deal.State, storagemarket.StorageDealError)
			require.Equal(t, deal.Message, "error attempting to close stream: unable to close")
		})
	})
}

type executor func(t *testing.T,
	node storagemarket.StorageClientNode,
	dealStreamErr error,
	dealStream smnet.StorageDealStream,
	closeStreamErr error,
	dealInspector func(deal storagemarket.ClientDeal))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc clientstates.ClientStateEntryFunc,
	initialState storagemarket.StorageDealStatus,
	clientDealProposal *market.ClientDealProposal) executor {
	return func(t *testing.T,
		node storagemarket.StorageClientNode,
		dealStreamErr error,
		dealStream smnet.StorageDealStream,
		closeStreamErr error,
		dealInspector func(deal storagemarket.ClientDeal)) {
		dealState, err := tut.MakeTestClientDeal(initialState, clientDealProposal)
		require.NoError(t, err)
		environment := &fakeEnvironment{node, dealStream, dealStreamErr, closeStreamErr}
		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(*dealState)
	}
}

type fakeEnvironment struct {
	node           storagemarket.StorageClientNode
	dealStream     smnet.StorageDealStream
	dealStreamErr  error
	closeStreamErr error
}

func (fe *fakeEnvironment) Node() storagemarket.StorageClientNode {
	return fe.node
}

func (fe *fakeEnvironment) DealStream(proposalCid cid.Cid) (smnet.StorageDealStream, error) {
	if fe.dealStreamErr == nil {
		return fe.dealStream, nil
	}
	return nil, fe.dealStreamErr
}

func (fe *fakeEnvironment) CloseStream(proposalCid cid.Cid) error {
	return fe.closeStreamErr
}
