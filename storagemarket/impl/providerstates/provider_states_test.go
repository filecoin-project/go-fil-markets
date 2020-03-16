package providerstates_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestValidateDealProposal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runValidateDealProposal := makeExecutor(ctx, eventProcessor, providerstates.ValidateDealProposal, storagemarket.StorageDealValidating)
	otherAddr, err := address.NewActorAddress([]byte("applesauce"))
	require.NoError(t, err)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealProposalAccepted)
			},
		},
		"verify signature fails": {
			nodeParams: nodeParams{
				VerifySignatureFails: true,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: verifying StorageDealProposal: could not verify signature")
			},
		},
		"provider address does not match": {
			environmentParams: environmentParams{
				Address: otherAddr,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: incorrect provider for deal")
			},
		},
		"MostRecentStateID errors": {
			nodeParams: nodeParams{
				MostRecentStateIDError: errors.New("couldn't get id"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error calling node: getting most recent state id: couldn't get id")
			},
		},
		"Height < StartEpoch": {
			nodeParams: nodeParams{
				Height: abi.ChainEpoch(250),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: deal proposal already expired")
			},
		},
		"PricePerEpoch too low": {
			dealParams: dealParams{
				StoragePricePerEpoch: abi.NewTokenAmount(5000),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: storage price per epoch less than asking price: 5000 < 9765")
			},
		},
		"PieceSize < MinPieceSize": {
			dealParams: dealParams{
				PieceSize: abi.PaddedPieceSize(128),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: piece size less than minimum required size: 128 < 256")
			},
		},
		"Get balance error": {
			nodeParams: nodeParams{
				ClientMarketBalanceError: errors.New("could not get balance"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error calling node: getting client market balance failed: could not get balance")
			},
		},
		"Not enough funds": {
			nodeParams: nodeParams{
				ClientMarketBalance: abi.NewTokenAmount(150 * 10000),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: clientMarketBalance.Available too small")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runValidateDealProposal(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestTransferData(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runTransferData := makeExecutor(ctx, eventProcessor, providerstates.TransferData, storagemarket.StorageDealProposalAccepted)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealTransferring)
			},
		},
		"manual transfer": {
			dealParams: dealParams{
				DataRef: &storagemarket.DataRef{
					TransferType: storagemarket.TTManual,
				},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealWaitingForData)
			},
		},
		"data transfer failure": {
			environmentParams: environmentParams{
				DataTransferError: errors.New("could not initiate"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error transferring data: failed to open pull data channel: could not initiate")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runTransferData(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestVerifyData(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	expPath := filestore.Path("applesauce.txt")
	runVerifyData := makeExecutor(ctx, eventProcessor, providerstates.VerifyData, storagemarket.StorageDealVerifyData)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			environmentParams: environmentParams{
				Path: expPath,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealPublishing)
				require.Equal(t, deal.PiecePath, expPath)
			},
		},
		"generate piece CID fails": {
			environmentParams: environmentParams{
				GenerateCommPError: errors.New("could not generate CommP"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "generating piece committment: could not generate CommP")
			},
		},
		"piece CIDs do not match": {
			environmentParams: environmentParams{
				PieceCid: tut.GenerateCids(1)[0],
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "deal rejected: proposal CommP doesn't match calculated CommP")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runVerifyData(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestPublishDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runPublishDeal := makeExecutor(ctx, eventProcessor, providerstates.PublishDeal, storagemarket.StorageDealPublishing)
	expDealID := abi.DealID(rand.Uint64())
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			nodeParams: nodeParams{
				PublishDealID: expDealID,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealStaged)
				require.Equal(t, deal.DealID, expDealID)
			},
		},
		"get miner worker fails": {
			nodeParams: nodeParams{
				MinerWorkerError: errors.New("could not get worker"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error calling node: looking up miner worker: could not get worker")
			},
		},
		"ensureFunds errors": {
			nodeParams: nodeParams{
				EnsureFundsError: errors.New("not enough funds"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error calling node: ensuring funds: not enough funds")
			},
		},
		"PublishDealsErrors errors": {
			nodeParams: nodeParams{
				PublishDealsError: errors.New("could not post to chain"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealFailing)
				require.Equal(t, deal.Message, "error calling node: publishing deal: could not post to chain")
			},
		},
		"SendSignedResponse errors": {
			environmentParams: environmentParams{
				SendSignedResponseError: errors.New("could not send"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "sending response to deal: could not send")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runPublishDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestHandoffDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runHandoffDeal := makeExecutor(ctx, eventProcessor, providerstates.HandoffDeal, storagemarket.StorageDealStaged)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealSealing)
			},
		},
		"opening file errors": {
			dealParams: dealParams{
				PiecePath: filestore.Path("missing.txt"),
			},
			environmentParams: environmentParams{
				OpenFileError: errors.New("file not found"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "accessing file store: reading piece at path missing.txt: file not found")
			},
		},
		"OnDealComplete errors": {
			nodeParams: nodeParams{
				OnDealCompleteError: errors.New("failed building sector"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "handing off deal to node: failed building sector")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runHandoffDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestVerifyDealActivated(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, providerstates.VerifyDealActivated, storagemarket.StorageDealSealing)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealActive)
			},
		},
		"sync error": {
			nodeParams: nodeParams{
				DealCommittedSyncError: errors.New("couldn't check deal commitment"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "error activating deal: couldn't check deal commitment")
			},
		},
		"async error": {
			nodeParams: nodeParams{
				DealCommittedAsyncError: errors.New("deal did not appear on chain"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "error activating deal: deal did not appear on chain")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runVerifyDealActivated(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestRecordPieceInfo(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runRecordPieceInfo := makeExecutor(ctx, eventProcessor, providerstates.RecordPieceInfo, storagemarket.StorageDealActive)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealCompleted)
			},
		},
		"file deletion errors": {
			dealParams: dealParams{
				PiecePath: filestore.Path("readonly.txt"),
			},
			environmentParams: environmentParams{
				DeleteFileError: errors.New("file is read only"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "accessing file store: deleting piece at path readonly.txt: file is read only")
			},
		},
		"locate piece fails": {
			dealParams: dealParams{
				DealID: abi.DealID(1234),
			},
			nodeParams: nodeParams{
				LocatePieceForDealWithinSectorError: errors.New("could not find piece"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "locating piece for deal ID 1234 in sector: could not find piece")
			},
		},
		"add piece block locations errors": {
			environmentParams: environmentParams{
				AddPieceBlockLocationsError: errors.New("could not add block locations"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "accessing piece store: adding piece block locations: could not add block locations")
			},
		},
		"add deal for piece errors": {
			environmentParams: environmentParams{
				AddDealForPieceError: errors.New("could not add deal info"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "accessing piece store: adding deal info for piece: could not add deal info")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runRecordPieceInfo(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

func TestFailDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runFailDeal := makeExecutor(ctx, eventProcessor, providerstates.FailDeal, storagemarket.StorageDealFailing)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
			},
		},
		"SendSignedResponse errors": {
			environmentParams: environmentParams{
				SendSignedResponseError: errors.New("could not send"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal) {
				require.Equal(t, deal.State, storagemarket.StorageDealError)
				require.Equal(t, deal.Message, "sending response to deal: could not send")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runFailDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.dealInspector)
		})
	}
}

// all of these default parameters are setup to allow a deal to complete each handler with no errors
var defaultHeight = abi.ChainEpoch(50)
var defaultTipSetToken = []byte{1, 2, 3}
var defaultStoragePricePerEpoch = abi.NewTokenAmount(10000)
var defaultPieceSize = abi.PaddedPieceSize(1048576)
var defaultStartEpoch = abi.ChainEpoch(200)
var defaultEndEpoch = abi.ChainEpoch(400)
var defaultPieceCid = tut.GenerateCids(1)[0]
var defaultPath = filestore.Path("file.txt")
var defaultClientAddress = address.TestAddress
var defaultProviderAddress = address.TestAddress2
var defaultMinerAddr, _ = address.NewActorAddress([]byte("miner"))
var defaultClientCollateral = abi.NewTokenAmount(0)
var defaultProviderCollateral = abi.NewTokenAmount(0)
var defaultDataRef = storagemarket.DataRef{
	Root:         tut.GenerateCids(1)[0],
	TransferType: storagemarket.TTGraphsync,
}
var defaultClientMarketBalance = abi.NewTokenAmount(200 * 10000)

var defaultAsk = storagemarket.StorageAsk{
	Price:        abi.NewTokenAmount(10000000),
	MinPieceSize: abi.PaddedPieceSize(256),
}

type nodeParams struct {
	MinerAddr                           address.Address
	MinerWorkerError                    error
	EnsureFundsError                    error
	Height                              abi.ChainEpoch
	TipSetToken                         shared.TipSetToken
	ClientMarketBalance                 abi.TokenAmount
	ClientMarketBalanceError            error
	VerifySignatureFails                bool
	MostRecentStateIDError              error
	PieceLength                         uint64
	PieceSectorID                       uint64
	PublishDealID                       abi.DealID
	PublishDealsError                   error
	OnDealCompleteError                 error
	LocatePieceForDealWithinSectorError error
	DealCommittedSyncError              error
	DealCommittedAsyncError             error
}

type dealParams struct {
	PiecePath            filestore.Path
	DealID               abi.DealID
	DataRef              *storagemarket.DataRef
	StoragePricePerEpoch abi.TokenAmount
	PieceSize            abi.PaddedPieceSize
	StartEpoch           abi.ChainEpoch
	EndEpoch             abi.ChainEpoch
}

type environmentParams struct {
	Address                     address.Address
	Ask                         storagemarket.StorageAsk
	DataTransferError           error
	PieceCid                    cid.Cid
	Path                        filestore.Path
	GenerateCommPError          error
	SendSignedResponseError     error
	DisconnectError             error
	File                        filestore.File
	OpenFileError               error
	DeleteFileError             error
	AddDealForPieceError        error
	AddPieceBlockLocationsError error
}

type executor func(t *testing.T,
	node nodeParams,
	params environmentParams,
	dealParams dealParams,
	dealInspector func(t *testing.T, deal storagemarket.MinerDeal))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc providerstates.ProviderStateEntryFunc,
	initialState storagemarket.StorageDealStatus) executor {
	return func(t *testing.T,
		nodeParams nodeParams,
		params environmentParams,
		dealParams dealParams,
		dealInspector func(t *testing.T, deal storagemarket.MinerDeal)) {

		smstate := testnodes.NewStorageMarketState()
		if nodeParams.Height != abi.ChainEpoch(0) {
			smstate.Epoch = nodeParams.Height
			smstate.TipSetToken = nodeParams.TipSetToken
		} else {
			smstate.Epoch = defaultHeight
			smstate.TipSetToken = defaultTipSetToken
		}
		if !nodeParams.ClientMarketBalance.Nil() {
			smstate.AddFunds(defaultClientAddress, nodeParams.ClientMarketBalance)
		} else {
			smstate.AddFunds(defaultClientAddress, defaultClientMarketBalance)
		}

		common := testnodes.FakeCommonNode{
			SMState:              smstate,
			GetChainHeadError:    nodeParams.MostRecentStateIDError,
			GetBalanceError:      nodeParams.ClientMarketBalanceError,
			VerifySignatureFails: nodeParams.VerifySignatureFails,
			EnsureFundsError:     nodeParams.EnsureFundsError,
		}

		node := &testnodes.FakeProviderNode{
			FakeCommonNode:                      common,
			MinerAddr:                           nodeParams.MinerAddr,
			MinerWorkerError:                    nodeParams.MinerWorkerError,
			PieceLength:                         nodeParams.PieceLength,
			PieceSectorID:                       nodeParams.PieceSectorID,
			PublishDealID:                       nodeParams.PublishDealID,
			PublishDealsError:                   nodeParams.PublishDealsError,
			OnDealCompleteError:                 nodeParams.OnDealCompleteError,
			LocatePieceForDealWithinSectorError: nodeParams.LocatePieceForDealWithinSectorError,
			DealCommittedSyncError:              nodeParams.DealCommittedSyncError,
			DealCommittedAsyncError:             nodeParams.DealCommittedAsyncError,
		}

		if nodeParams.MinerAddr == address.Undef {
			node.MinerAddr = defaultMinerAddr
		}

		proposal := market.DealProposal{
			PieceCID:             defaultPieceCid,
			PieceSize:            defaultPieceSize,
			Client:               defaultClientAddress,
			Provider:             defaultProviderAddress,
			StartEpoch:           defaultStartEpoch,
			EndEpoch:             defaultEndEpoch,
			StoragePricePerEpoch: defaultStoragePricePerEpoch,
			ProviderCollateral:   defaultProviderCollateral,
			ClientCollateral:     defaultClientCollateral,
		}
		if !dealParams.StoragePricePerEpoch.Nil() {
			proposal.StoragePricePerEpoch = dealParams.StoragePricePerEpoch
		}
		if dealParams.StartEpoch != abi.ChainEpoch(0) {
			proposal.StartEpoch = dealParams.StartEpoch
		}
		if dealParams.EndEpoch != abi.ChainEpoch(0) {
			proposal.EndEpoch = dealParams.EndEpoch
		}
		if dealParams.PieceSize != abi.PaddedPieceSize(0) {
			proposal.PieceSize = dealParams.PieceSize
		}
		signedProposal := &market.ClientDealProposal{
			Proposal:        proposal,
			ClientSignature: *tut.MakeTestSignature(),
		}
		dataRef := &defaultDataRef
		if dealParams.DataRef != nil {
			dataRef = dealParams.DataRef
		}
		dealState, err := tut.MakeTestMinerDeal(initialState,
			signedProposal, dataRef)
		require.NoError(t, err)
		if dealParams.PiecePath != filestore.Path("") {
			dealState.PiecePath = dealParams.PiecePath
		}
		if dealParams.DealID != abi.DealID(0) {
			dealState.DealID = dealParams.DealID
		}
		environment := &fakeEnvironment{
			address:                     params.Address,
			node:                        node,
			ask:                         params.Ask,
			dataTransferError:           params.DataTransferError,
			pieceCid:                    params.PieceCid,
			path:                        params.Path,
			generateCommPError:          params.GenerateCommPError,
			sendSignedResponseError:     params.SendSignedResponseError,
			disconnectError:             params.DisconnectError,
			openFileError:               params.OpenFileError,
			deleteFileError:             params.DeleteFileError,
			addDealForPieceError:        params.AddDealForPieceError,
			addPieceBlockLocationsError: params.AddPieceBlockLocationsError,
		}
		if environment.pieceCid == cid.Undef {
			environment.pieceCid = defaultPieceCid
		}
		if environment.path == filestore.Path("") {
			environment.path = defaultPath
		}
		if environment.address == address.Undef {
			environment.address = defaultProviderAddress
		}
		if environment.ask == storagemarket.StorageAskUndefined {
			environment.ask = defaultAsk
		}

		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(t, *dealState)
	}
}

type fakeEnvironment struct {
	address                     address.Address
	node                        storagemarket.StorageProviderNode
	ask                         storagemarket.StorageAsk
	dataTransferError           error
	pieceCid                    cid.Cid
	path                        filestore.Path
	generateCommPError          error
	sendSignedResponseError     error
	disconnectError             error
	openFileError               error
	deleteFileError             error
	addDealForPieceError        error
	addPieceBlockLocationsError error
}

func (fe *fakeEnvironment) Address() address.Address {
	return fe.address
}

func (fe *fakeEnvironment) Node() storagemarket.StorageProviderNode {
	return fe.node
}

func (fe *fakeEnvironment) Ask() storagemarket.StorageAsk {
	return fe.ask
}

func (fe *fakeEnvironment) StartDataTransfer(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) error {
	return fe.dataTransferError
}

func (fe *fakeEnvironment) GeneratePieceCommitmentToFile(payloadCid cid.Cid, selector ipld.Node) (cid.Cid, filestore.Path, error) {
	return fe.pieceCid, fe.path, fe.generateCommPError
}

func (fe *fakeEnvironment) SendSignedResponse(ctx context.Context, response *network.Response) error {
	return fe.sendSignedResponseError
}

func (fe *fakeEnvironment) Disconnect(proposalCid cid.Cid) error {
	return fe.disconnectError
}

func (fe *fakeEnvironment) OpenFile(path filestore.Path) (filestore.File, error) {
	return &fakeFile{}, fe.openFileError
}

func (fe *fakeEnvironment) DeleteFile(path filestore.Path) error {
	return fe.deleteFileError
}

func (fe *fakeEnvironment) AddDealForPiece(pieceCID cid.Cid, dealInfo piecestore.DealInfo) error {
	return fe.addDealForPieceError
}

func (fe *fakeEnvironment) AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]piecestore.BlockLocation) error {
	return fe.addPieceBlockLocationsError
}

type fakeFile struct {
}

func (f *fakeFile) Path() filestore.Path {
	panic("not implemented")
}

func (f *fakeFile) OsPath() filestore.OsPath {
	panic("not implemented")
}

func (f *fakeFile) Size() int64 {
	return 400
}

func (f *fakeFile) Close() error {
	panic("not implemented")
}

func (f *fakeFile) Read(p []byte) (n int, err error) {
	panic("not implemented")
}

func (f *fakeFile) Write(p []byte) (n int, err error) {
	panic("not implemented")
}

func (f *fakeFile) Seek(offset int64, whence int) (int64, error) {
	panic("not implemented")
}
