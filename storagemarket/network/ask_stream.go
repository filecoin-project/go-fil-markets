package network

import (
	"bufio"
	"encoding/hex"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"
)

type askStream struct {
	p        peer.ID
	rw       network.MuxedStream
	buffered *bufio.Reader
}

var _ StorageAskStream = (*askStream)(nil)

func (as *askStream) ReadAskRequest() (AskRequest, error) {
	var a AskRequest

	if err := a.UnmarshalCBOR(as.buffered); err != nil {
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
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	origBytes, err := cborutil.Dump(resp.Ask.Ask)
	if err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	ask := resp.Ask.Ask

	sigb, err := resp.Ask.Signature.MarshalBinary()
	if err != nil {
		log.Warn(err)
		return AskResponseUndefined, nil, err
	}

	log.Infof("Ask response receieved on network:\n"+
		"New storage ask:\nPrice: %s\nVerifiedPrice: %s\nTimestamp: %s\n"+
		"Expiry: %d\nMiner: %s\nSeqNo: %d\nMinPieceSize: %d\n"+
		"MaxPieceSize: %d", ask.Price.String(), ask.VerifiedPrice.String(),
		ask.Timestamp.String(), ask.Expiry, ask.Miner.String(), ask.SeqNo,
		ask.MinPieceSize, ask.MaxPieceSize)

	log.Infof("Ask response receieved on network\n"+
		"Crypto Signature: %s", hex.EncodeToString(sigb))

	log.Infof("Ask response receieved on network\n"+
		"Ask Bytes: %s", hex.EncodeToString(origBytes))

	return resp, origBytes, nil
}

func (as *askStream) WriteAskResponse(qr AskResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(as.rw, &qr)
}

func (as *askStream) Close() error {
	return as.rw.Close()
}
