package network

import (
	"bufio"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
)

type dealStream struct {
	p        peer.ID
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ StorageDealStream = (*dealStream)(nil)

func (d *dealStream) ReadDealProposal() (storagemarket.ProposalRequest, error) {
	var ds storagemarket.ProposalRequest

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return storagemarket.ProposalRequestUndefined, err
	}
	return ds, nil
}

func (d *dealStream) WriteDealProposal(dp storagemarket.ProposalRequest) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *dealStream) ReadDealResponse() (storagemarket.SignedResponse, error) {
	var dr storagemarket.SignedResponse

	if err := dr.UnmarshalCBOR(d.buffered); err != nil {
		return storagemarket.SignedResponseUndefined, err
	}
	return dr, nil
}

func (d *dealStream) WriteDealResponse(dr storagemarket.SignedResponse) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *dealStream) Close() error {
	return d.rw.Reset()
}

func (d *dealStream) RemotePeer() peer.ID {
	return d.p
}
