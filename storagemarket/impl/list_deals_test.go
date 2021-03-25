package storageimpl

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

func TestClientListDeals(t *testing.T) {
	ctx := context.Background()
	client := mkClient(t, ctx)

	// Add deals with different creation times, epochs & states
	cid1, _ := genAndAddDeal(t, client, time.Now(), abi.ChainEpoch(100), abi.ChainEpoch(200), storagemarket.StorageDealAwaitingPreCommit)
	cid2, _ := genAndAddDeal(t, client, time.Now().Add(1*time.Hour), abi.ChainEpoch(150), abi.ChainEpoch(300), storagemarket.StorageDealAcceptWait)
	cid3, _ := genAndAddDeal(t, client, time.Now().Add(2*time.Hour), abi.ChainEpoch(450), abi.ChainEpoch(500), storagemarket.StorageDealCheckForAcceptance)
	cid4, _ := genAndAddDeal(t, client, time.Now().Add(3*time.Hour), abi.ChainEpoch(400), abi.ChainEpoch(550), storagemarket.StorageDealError)

	tcs := map[string]struct {
		filter       []storagemarket.ListDealsPageParams
		expectedCids []cid.Cid
	}{
		"no params -> get all deals": {
			expectedCids: []cid.Cid{cid1, cid2, cid3, cid4},
		},
		"filter out errored deals": {
			filter: []storagemarket.ListDealsPageParams{
				{HideDealsInErrorState: true},
			},
			expectedCids: []cid.Cid{cid1, cid2, cid3},
		},
		"filter that includes errored deals": {
			filter: []storagemarket.ListDealsPageParams{
				{HideDealsInErrorState: false},
			},
			expectedCids: []cid.Cid{cid1, cid2, cid3, cid4},
		},
		"start time 30 minutes from now, will show 2, 3 & 4": {
			filter: []storagemarket.ListDealsPageParams{
				{
					CreationTimePageOffset: time.Now().Add(30 * time.Minute),
					HideDealsInErrorState:  false,
				},
			},
			expectedCids: []cid.Cid{cid2, cid3, cid4},
		},
		"start time 90 minutes from now, will show 3 & 4": {
			filter: []storagemarket.ListDealsPageParams{
				{
					CreationTimePageOffset: time.Now().Add(90 * time.Minute),
					HideDealsInErrorState:  false,
				},
			},
			expectedCids: []cid.Cid{cid3, cid4},
		},
		"start time 5 hours from now, will show none": {
			filter: []storagemarket.ListDealsPageParams{
				{
					CreationTimePageOffset: time.Now().Add(5 * time.Hour),
					HideDealsInErrorState:  false,
				},
			},
			expectedCids: nil,
		},
		"show all deals with start epoch > 300, will show cid3 & cid4": {
			filter: []storagemarket.ListDealsPageParams{
				{
					MinStartEpoch:         abi.ChainEpoch(300),
					HideDealsInErrorState: false,
				},
			},
			expectedCids: []cid.Cid{cid3, cid4},
		},
		"show deals with start epoch > 300 with creation time 150 minutes from now, will show cid4": {
			filter: []storagemarket.ListDealsPageParams{
				{
					CreationTimePageOffset: time.Now().Add(150 * time.Minute),
					MinStartEpoch:          abi.ChainEpoch(300),
					HideDealsInErrorState:  false,
				},
			},
			expectedCids: []cid.Cid{cid4},
		},
		"show deals with start epoch > 120 & end epoch < 501,  will show cid2 & cid3": {
			filter: []storagemarket.ListDealsPageParams{
				{
					MinStartEpoch:         abi.ChainEpoch(120),
					MaxEndEpoch:           abi.ChainEpoch(501),
					HideDealsInErrorState: false,
				},
			},
			expectedCids: []cid.Cid{cid2, cid3},
		},
		"show deals with start epoch > 120 & end epoch < 501 with creation time after 90 minutes,  will show cid3": {
			filter: []storagemarket.ListDealsPageParams{
				{
					CreationTimePageOffset: time.Now().Add(90 * time.Minute),
					MinStartEpoch:          abi.ChainEpoch(120),
					MaxEndEpoch:            abi.ChainEpoch(501),
					HideDealsInErrorState:  false,
				},
			},
			expectedCids: []cid.Cid{cid3},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			out, err := client.ListLocalDeals(ctx, tc.filter...)
			require.NoError(t, err)

			require.Len(t, out, len(tc.expectedCids))
			actualCids := make(map[cid.Cid]struct{})
			for _, d := range out {
				actualCids[d.ProposalCid] = struct{}{}
			}

			for _, ec := range tc.expectedCids {
				_, ok := actualCids[ec]
				require.True(t, ok)
			}
		})
	}

	// Show only 2 deals
	ds, err := client.ListLocalDeals(ctx, storagemarket.ListDealsPageParams{
		DealsPerPage:          2,
		HideDealsInErrorState: false,
	})
	require.NoError(t, err)
	require.Len(t, ds, 2)

	// Show ONLY 1 deal with start epoch > 120 & end epoch < 501
	f := storagemarket.ListDealsPageParams{
		DealsPerPage:          1,
		MinStartEpoch:         abi.ChainEpoch(120),
		MaxEndEpoch:           abi.ChainEpoch(501),
		HideDealsInErrorState: false,
	}
	ds, err = client.ListLocalDeals(ctx, f)
	require.NoError(t, err)
	require.Len(t, ds, 1)
	require.True(t, ds[0].ProposalCid.Equals(cid2) || ds[0].ProposalCid.Equals(cid3))
}

func TestPagination(t *testing.T) {
	ctx := context.Background()
	client := mkClient(t, ctx)

	// Add deals with different creation times, epochs & states
	cid1, _ := genAndAddDeal(t, client, time.Now(), abi.ChainEpoch(100), abi.ChainEpoch(200), storagemarket.StorageDealAwaitingPreCommit)
	cid2, _ := genAndAddDeal(t, client, time.Now().Add(1*time.Hour), abi.ChainEpoch(150), abi.ChainEpoch(300), storagemarket.StorageDealAcceptWait)
	cid3, _ := genAndAddDeal(t, client, time.Now().Add(2*time.Hour), abi.ChainEpoch(450), abi.ChainEpoch(500), storagemarket.StorageDealCheckForAcceptance)
	cid4, _ := genAndAddDeal(t, client, time.Now().Add(3*time.Hour), abi.ChainEpoch(400), abi.ChainEpoch(550), storagemarket.StorageDealError)

	// Get me the first two deals without any filter
	f := storagemarket.ListDealsPageParams{
		DealsPerPage: 2,
	}
	ds1, err := client.ListLocalDeals(ctx, f)
	require.NoError(t, err)
	require.Len(t, ds1, 2)
	require.Equal(t, cid1, ds1[0].ProposalCid)
	require.Equal(t, cid2, ds1[1].ProposalCid)

	// Get me two deals starting after the deals we just fetched
	f = storagemarket.ListDealsPageParams{
		DealsPerPage:           2,
		CreationTimePageOffset: ds1[1].CreationTime.Time(),
	}
	ds2, err := client.ListLocalDeals(ctx, f)
	require.NoError(t, err)
	require.Len(t, ds2, 2)
	require.Equal(t, cid3, ds2[0].ProposalCid)
	require.Equal(t, cid4, ds2[1].ProposalCid)
}

func mkClient(t *testing.T, ctx context.Context) *Client {
	deps := dependencies.NewDependenciesWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), testnodes.NewStorageMarketState(), "", noOpDelay,
		noOpDelay)
	clientDs := namespace.Wrap(deps.TestData.Ds1, datastore.NewKey("/deals/client"))
	client, err := NewClient(
		nil,
		deps.TestData.Bs1,
		deps.TestData.MultiStore1,
		deps.DTClient,
		deps.PeerResolver,
		clientDs,
		deps.ClientNode,
	)
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, client)

	return client
}

func genAndAddDeal(t *testing.T, client *Client, creationTime time.Time, startEpoch, endEpoch abi.ChainEpoch, state storagemarket.StorageDealStatus) (cid.Cid, *storagemarket.ClientDeal) {
	cid, deal := genDeal(t, creationTime, startEpoch, endEpoch, state)
	require.NoError(t, client.statemachines.Begin(cid, deal))
	return cid, deal
}

func genDeal(t *testing.T, creationTime time.Time, startEpoch, endEpoch abi.ChainEpoch, state storagemarket.StorageDealStatus) (cid.Cid, *storagemarket.ClientDeal) {
	prop := shared_testutil.MakeTestClientDealProposal()
	prop.Proposal.StartEpoch = startEpoch
	prop.Proposal.EndEpoch = endEpoch
	proposalNd, err := cborutil.AsIpld(prop)
	require.NoError(t, err)

	deal := &storagemarket.ClientDeal{
		ProposalCid:        proposalNd.Cid(),
		ClientDealProposal: *prop,
		State:              state,
		CreationTime:       cbg.CborTime(time.Unix(0, creationTime.UnixNano()).UTC()),
		MinerWorker:        address.TestAddress2,
	}

	return proposalNd.Cid(), deal
}
