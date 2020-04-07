package retrievalmarket

import (
	"bytes"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func DecodeNode(defnode *cbg.Deferred) (ipld.Node, error) {
	reader := bytes.NewReader(defnode.Raw)
	return dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
}
