package network

import (
	"bufio"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
)

type queryStream struct {
	p        peer.ID
	host     host.Host
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ StorageQueryStream = (*queryStream)(nil)

func (d *queryStream) ReadQueryRequest() (QueryRequest, error) {
	var q QueryRequest

	if err := q.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return QueryRequestUndefined, err
	}
	return q, nil
}

func (d *queryStream) WriteQueryRequest(q QueryRequest) error {
	return cborutil.WriteCborRPC(d.rw, &q)
}

func (d *queryStream) ReadQueryResponse() (QueryResponse, error) {
	var qr QueryResponse

	if err := qr.UnmarshalCBOR(d.buffered); err != nil {
		return QueryResponseUndefined, err
	}
	return qr, nil
}

func (d *queryStream) WriteQueryResponse(qr QueryResponse) error {
	return cborutil.WriteCborRPC(d.rw, &qr)
}

func (d *queryStream) Close() error {
	return d.rw.Close()
}

func (d *queryStream) RemotePeer() peer.ID {
	return d.p
}
