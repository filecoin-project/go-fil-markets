package storagemarket

import (
	"bytes"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

func TestMinerDealMarshallUnMarshall(t *testing.T) {
	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	old := MockOldMinerDeal{
		ProposalCid:   dummyCid,
		DealID:        10,
		CarV2FilePath: "myfilepath",
	}
	buf := new(bytes.Buffer)
	err = old.MarshalCBOR(buf)
	require.NoError(t, err)

	unmarshalled := MinerDeal{}
	err = unmarshalled.UnmarshalCBOR(buf)
	require.NoError(t, err)

	require.Equal(t, old.ProposalCid, unmarshalled.ProposalCid)
	require.Equal(t, old.DealID, unmarshalled.DealID)
	require.Equal(t, old.CarV2FilePath, unmarshalled.InboundCAR)
}
