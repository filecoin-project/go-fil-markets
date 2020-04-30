package retrievalmarket

import (
	"bytes"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func DecodeNode(defnode *cbg.Deferred) (ipld.Node, error) {
	reader := bytes.NewReader(defnode.Raw)
	nb := basicnode.Style.Any.NewBuilder()
	err := dagcbor.Decoder(nb, reader)
	if err != nil {
		return nil, err
	}
	return nb.Build(), nil
}
