package network

import (
	"bufio"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type QueryStream struct {
	p        peer.ID
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ RetrievalQueryStream = (*QueryStream)(nil)

func (qs *QueryStream) ReadQuery() (retrievalmarket.Query, error) {
	var q retrievalmarket.Query

	if err := q.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryUndefined, err

	}

	return q, nil
}

func (qs *QueryStream) WriteQuery(q retrievalmarket.Query) error {
	return cborutil.WriteCborRPC(qs.rw, &q)
}

func (qs *QueryStream) ReadQueryResponse() (retrievalmarket.QueryResponse, error) {
	var resp retrievalmarket.QueryResponse

	if err := resp.UnmarshalCBOR(qs.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return resp, nil
}

func (qs *QueryStream) WriteQueryResponse(qr retrievalmarket.QueryResponse) error {
	return cborutil.WriteCborRPC(qs.rw, &qr)
}

func (qs *QueryStream) Close() error {
	return qs.rw.Close()
}
