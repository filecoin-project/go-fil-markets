package retrievalmarket_test

import (
	"bytes"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

func TestParamsMarshalUnmarshal(t *testing.T) {
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	node := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	params := retrievalmarket.NewParamsV1(abi.NewTokenAmount(123), 456, 789, node)

	buf := new(bytes.Buffer)
	err := params.MarshalCBOR(buf)
	assert.NoError(t, err)

	unmarshalled := &retrievalmarket.Params{}
	err = unmarshalled.UnmarshalCBOR(buf)
	assert.NoError(t, err)

	assert.Equal(t, params, *unmarshalled)

	sel, err := dagcbor.Decoder(ipldfree.NodeBuilder(), bytes.NewBuffer(unmarshalled.Selector.Raw))
	assert.NoError(t, err)
	assert.Equal(t, sel, node)
}
