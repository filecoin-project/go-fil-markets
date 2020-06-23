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

func (d *queryStream) ReadQueryRequest() (SignedQueryRequest, error) {
	var q SignedQueryRequest

	if err := q.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return QueryRequestUndefined, err
	}
	return q, nil
}

func (d *queryStream) WriteQueryRequest(q SignedQueryRequest) error {
	return cborutil.WriteCborRPC(d.rw, &q)
}

func (d *queryStream) ReadQueryResponse() (SignedQueryResponse, error) {
	var qr SignedQueryResponse

	if err := qr.UnmarshalCBOR(d.buffered); err != nil {
		return SignedQueryResponse{}, err
	}
	return qr, nil
}

func (d *queryStream) WriteQueryResponse(qr SignedQueryResponse) error {
	return cborutil.WriteCborRPC(d.rw, &qr)
}

func (d *queryStream) Close() error {
	return d.rw.Close()
}

func (d *queryStream) RemotePeer() peer.ID {
	return d.p
}
