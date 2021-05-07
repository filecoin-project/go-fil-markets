package retrievalmarket_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/libp2p/go-libp2p-core/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestParamsMarshalUnmarshal(t *testing.T) {
	pieceCid := tut.GenerateCids(1)[0]

	allSelector := shared.AllSelector()
	params, err := retrievalmarket.NewParamsV1(abi.NewTokenAmount(123), 456, 789, allSelector, &pieceCid, big.Zero())
	assert.NoError(t, err)

	buf := new(bytes.Buffer)
	err = params.MarshalCBOR(buf)
	assert.NoError(t, err)

	unmarshalled := &retrievalmarket.Params{}
	err = unmarshalled.UnmarshalCBOR(buf)
	assert.NoError(t, err)

	assert.Equal(t, params, *unmarshalled)

	nb := basicnode.Prototype.Any.NewBuilder()
	err = dagcbor.Decoder(nb, bytes.NewBuffer(unmarshalled.Selector.Raw))
	assert.NoError(t, err)
	sel := nb.Build()
	assert.Equal(t, sel, allSelector)
}

func TestPricingInputMarshalUnmarshalJSON(t *testing.T) {
	pid := test.RandPeerIDFatal(t)

	in := retrievalmarket.PricingInput{
		PayloadCID:   tut.GenerateCids(1)[0],
		PieceCID:     tut.GenerateCids(1)[0],
		PieceSize:    abi.UnpaddedPieceSize(100),
		Client:       pid,
		VerifiedDeal: true,
		Unsealed:     true,
		CurrentAsk: retrievalmarket.Ask{
			PricePerByte:            big.Zero(),
			UnsealPrice:             big.Zero(),
			PaymentInterval:         0,
			PaymentIntervalIncrease: 0,
		},
	}

	bz, err := json.Marshal(in)
	require.NoError(t, err)

	resp2 := retrievalmarket.PricingInput{}
	require.NoError(t, json.Unmarshal(bz, &resp2))

	require.Equal(t, in, resp2)
}
