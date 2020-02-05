package network

import (
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

type askStream struct {
	p  peer.ID
	rw mux.MuxedStream
}

var _ StorageAskStream = (*askStream)(nil)

func (as *askStream) ReadAskRequest() (AskRequest, error) {
	var a AskRequest

	if err := a.UnmarshalCBOR(as.rw); err != nil {
		log.Warn(err)
		return AskRequestUndefined, err

	}

	return a, nil
}

func (as *askStream) WriteAskRequest(q AskRequest) error {
	return cborutil.WriteCborRPC(as.rw, &q)
}

func (as *askStream) ReadAskResponse() (AskResponse, error) {
	var resp AskResponse

	if err := resp.UnmarshalCBOR(as.rw); err != nil {
		log.Warn(err)
		return AskResponseUndefined, err
	}

	return resp, nil
}

func (as *askStream) WriteAskResponse(qr AskResponse) error {
	return cborutil.WriteCborRPC(as.rw, &qr)
}

func (as *askStream) Close() error {
	return as.rw.Close()
}
