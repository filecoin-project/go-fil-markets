package retrievalmarket_test

import (
	"bytes"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestParamsMarshalUnmarshal(t *testing.T) {
	pieceCid := tut.GenerateCids(1)[0]

	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	node := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	params := retrievalmarket.NewParamsV1(abi.NewTokenAmount(123), 456, 789, node, &pieceCid)

	buf := new(bytes.Buffer)
	err := params.MarshalCBOR(buf)
	assert.NoError(t, err)

	unmarshalled := &retrievalmarket.Params{}
	err = unmarshalled.UnmarshalCBOR(buf)
	assert.NoError(t, err)

	assert.Equal(t, params, *unmarshalled)

	nb := basicnode.Style.Any.NewBuilder()
	err = dagcbor.Decoder(nb, bytes.NewBuffer(unmarshalled.Selector.Raw))
	assert.NoError(t, err)
	sel := nb.Build()
	assert.Equal(t, sel, node)
}
