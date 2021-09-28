package network

import (
	"bufio"

	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

// askStream implements the RetrievalAskStream interface.
// It provides reads and writes ask requests and responses to a stream as CBOR
type askStream struct {
	p        peer.ID
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ RetrievalAskStream = (*askStream)(nil)

func (as *askStream) ReadAskRequest() (AskRequest, error) {
	var a AskRequest

	if err := a.UnmarshalCBOR(as.buffered); err != nil {
		err = xerrors.Errorf("unmarshalling ask request buffer: %w", err)
		log.Warn(err)
		return AskRequestUndefined, err
	}

	return a, nil
}

func (as *askStream) WriteAskRequest(q AskRequest) error {
	return cborutil.WriteCborRPC(as.rw, &q)
}

func (as *askStream) ReadAskResponse() (AskResponse, []byte, error) {
	var resp AskResponse

	if err := resp.UnmarshalCBOR(as.buffered); err != nil {
		err = xerrors.Errorf("unmarshalling ask response buffer: %w", err)
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(resp.Ask.Ask)
	if err != nil {
		err = xerrors.Errorf("remarshalling ask response: %w", err)
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}
	return resp, origBytes, nil
}

func (as *askStream) WriteAskResponse(qr AskResponse) error {
	return cborutil.WriteCborRPC(as.rw, &qr)
}

func (as *askStream) Close() error {
	return as.rw.Close()
}
