package network

import (
	"bufio"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
)

type askStream struct {
	p        peer.ID
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ StorageAskStream = (*askStream)(nil)

func (as *askStream) ReadAskRequest() (storagemarket.AskRequest, error) {
	var a storagemarket.AskRequest

	if err := a.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return storagemarket.AskRequestUndefined, err

	}

	return a, nil
}

func (as *askStream) WriteAskRequest(q storagemarket.AskRequest) error {
	return cborutil.WriteCborRPC(as.rw, &q)
}

func (as *askStream) ReadAskResponse() (storagemarket.AskResponse, error) {
	var resp storagemarket.AskResponse

	if err := resp.UnmarshalCBOR(as.buffered); err != nil {
		log.Warn(err)
		return storagemarket.AskResponseUndefined, err
	}

	return resp, nil
}

func (as *askStream) WriteAskResponse(qr storagemarket.AskResponse) error {
	return cborutil.WriteCborRPC(as.rw, &qr)
}

func (as *askStream) Close() error {
	return as.rw.Close()
}
