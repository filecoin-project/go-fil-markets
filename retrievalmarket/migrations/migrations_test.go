package migrations

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/big"

	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs/go-cid"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"

	"github.com/stretchr/testify/require"

	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	"github.com/filecoin-project/go-statemachine/fsm"
)

func TestClientStateMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a v0 client deal state
	dealID := retrievalmarket.DealID(1)
	storeID := multistore.StoreID(1)
	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
	dealState := ClientDealState0{
		DealProposal0: DealProposal0{
			PayloadCID: dummyCid,
			ID:         dealID,
			Params0: Params0{
				PieceCID:     &dummyCid,
				PricePerByte: abi.NewTokenAmount(0),
				UnsealPrice:  abi.NewTokenAmount(0),
			},
		},
		TotalFunds:       abi.NewTokenAmount(0),
		ClientWallet:     tutils.NewActorAddr(t, "client"),
		MinerWallet:      tutils.NewActorAddr(t, "miner"),
		TotalReceived:    0,
		CurrentInterval:  10,
		BytesPaidFor:     0,
		PaymentRequested: abi.NewTokenAmount(0),
		FundsSpent:       abi.NewTokenAmount(0),
		Status:           retrievalmarket.DealStatusNew,
		Sender:           peer.ID("sender"),
		UnsealFundsPaid:  big.Zero(),
		StoreID:          &storeID,
	}
	dealStateWithChannelID := dealState
	chid := datatransfer.ChannelID{
		Initiator: "initiator",
		Responder: "responder",
		ID:        1,
	}
	dealStateWithChannelID.ChannelID = chid

	testCases := []struct {
		name         string
		dealState0   *ClientDealState0
		expChannelID *datatransfer.ChannelID
	}{{
		name:         "from v0 - v2 with channel ID",
		dealState0:   &dealState,
		expChannelID: nil,
	}, {
		name:         "from v0 - v2 with no channel ID",
		dealState0:   &dealStateWithChannelID,
		expChannelID: &chid,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds := dss.MutexWrap(datastore.NewMapDatastore())

			// Store the v0 client deal state to the datastore
			stateMachines0, err := fsm.New(ds, fsm.Parameters{
				Environment:     &mockEnvironment{},
				StateType:       ClientDealState0{},
				StateKeyField:   "Status",
				Events:          fsm.Events{},
				StateEntryFuncs: fsm.StateEntryFuncs{},
				FinalityStates:  []fsm.StateKey{},
			})
			require.NoError(t, err)

			err = stateMachines0.Begin(dealID, tc.dealState0)
			require.NoError(t, err)

			// Prepare to run migration to v2 datastore
			retrievalMigrations, err := ClientMigrations.Build()
			require.NoError(t, err)

			stateMachines, migrateStateMachines, err := versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
				Environment:     &mockEnvironment{},
				StateType:       retrievalmarket.ClientDealState{},
				StateKeyField:   "Status",
				Events:          clientstates.ClientEvents,
				StateEntryFuncs: clientstates.ClientStateEntryFuncs,
				FinalityStates:  clientstates.ClientFinalityStates,
			}, retrievalMigrations, "2")

			// Run migration to v2 datastore
			err = migrateStateMachines(ctx)
			require.NoError(t, err)

			var states []retrievalmarket.ClientDealState
			stateMachines.List(&states)

			require.Len(t, states, 1)
			if tc.expChannelID == nil {
				// Ensure that the channel ID is nil if it was not explicitly defined
				require.Nil(t, states[0].ChannelID)
			} else {
				// Ensure that the channel ID is correct if it was defined
				require.Equal(t, chid, *states[0].ChannelID)
			}
		})
	}
}

type mockEnvironment struct {
}

func (e *mockEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return nil
}

func (e *mockEnvironment) OpenDataTransfer(ctx context.Context, to peer.ID, proposal *retrievalmarket.DealProposal, legacy bool) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

func (e *mockEnvironment) SendDataTransferVoucher(_ context.Context, _ datatransfer.ChannelID, _ *retrievalmarket.DealPayment, _ bool) error {
	return nil
}

func (e *mockEnvironment) CloseDataTransfer(_ context.Context, _ datatransfer.ChannelID) error {
	return nil
}
