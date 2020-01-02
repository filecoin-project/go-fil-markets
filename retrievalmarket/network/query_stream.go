package network

import (
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	"github.com/libp2p/go-libp2p-core/peer"
	"io"
)

type QueryStream struct {
	p  peer.ID
	rw io.ReadWriter
}

var _ RetrievalQueryStream = (*QueryStream)(nil)

func NewQueryStream(p peer.ID, rw io.ReadWriter) *QueryStream {
	return &QueryStream{p, rw}
}

func (qs *QueryStream) ReadQuery() (retrievalmarket.Query, error) {
	var q retrievalmarket.Query

	if err := q.UnmarshalCBOR(qs.rw); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryUndefined, err

	}

	return q, nil
}

func (qs *QueryStream) WriteQuery(q retrievalmarket.Query) error {
	return cborutil.WriteCborRPC(qs.rw, q)
}

func (qs *QueryStream) ReadQueryResponse() (retrievalmarket.QueryResponse, error) {
	var resp retrievalmarket.QueryResponse

	if err := resp.UnmarshalCBOR(qs.rw); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return resp, nil
}

func (qs *QueryStream) WriteQueryResponse(qr retrievalmarket.QueryResponse) error {
	return cborutil.WriteCborRPC(qs.rw, qr)
}
