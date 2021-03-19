package storagemarket_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

func TestDealStagesMarshalUnmarshal(t *testing.T) {
	prop := shared_testutil.MakeTestClientDealProposal()
	deal, err := shared_testutil.MakeTestClientDeal(storagemarket.StorageDealUnknown, prop, true)
	require.NoError(t, err)

	oldDeal := &storagemarket.OldClientDeal{
		ClientDealProposal: deal.ClientDealProposal,
		ProposalCid:        deal.ProposalCid,
		AddFundsCid:        deal.AddFundsCid,
		State:              deal.State,
		Miner:              deal.Miner,
		MinerWorker:        deal.MinerWorker,
		DealID:             deal.DealID,
		DataRef:            deal.DataRef,
		Message:            deal.Message,
		PublishMessage:     deal.PublishMessage,
		SlashEpoch:         deal.SlashEpoch,
		PollRetryCount:     deal.PollRetryCount,
		PollErrorCount:     deal.PollErrorCount,
		FastRetrieval:      deal.FastRetrieval,
		StoreID:            deal.StoreID,
		FundsReserved:      deal.FundsReserved,
		CreationTime:       deal.CreationTime,
		TransferChannelID:  deal.TransferChannelID,
		SectorNumber:       deal.SectorNumber,
	}

	buf := new(bytes.Buffer)
	err = oldDeal.MarshalCBOR(buf)
	require.NoError(t, err)

	unmarshalled := &storagemarket.ClientDeal{}
	err = unmarshalled.UnmarshalCBOR(buf)
	require.NoError(t, err)

	unmarshalled.DealStages.GetStage("none")
	unmarshalled.DealStages.AddStageLog("MyStage", "desc", "duration", "msg")
}
