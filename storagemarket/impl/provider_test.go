package storageimpl_test

import (
	"testing"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"

	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
)

func TestConfigure(t *testing.T) {
	p := &storageimpl.Provider{}

	assert.False(t, p.UniversalRetrievalEnabled())
	assert.Equal(t, abi.ChainEpoch(0), p.DealAcceptanceBuffer())

	p.Configure(
		storageimpl.EnableUniversalRetrieval(),
		storageimpl.DealAcceptanceBuffer(abi.ChainEpoch(123)),
	)

	assert.True(t, p.UniversalRetrievalEnabled())
	assert.Equal(t, abi.ChainEpoch(123), p.DealAcceptanceBuffer())
}

type fakeDTValidator struct{}

func (f fakeDTValidator) ValidatePush(_ peer.ID, _ datatransfer.Voucher, _ cid.Cid, _ ipld.Node) error {
	return nil
}
func (f fakeDTValidator) ValidatePull(_ peer.ID, _ datatransfer.Voucher, _ cid.Cid, _ ipld.Node) error {
	return nil
}

var _ datatransfer.RequestValidator = (*fakeDTValidator)(nil)
