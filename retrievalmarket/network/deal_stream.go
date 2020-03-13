package network

import (
	"bufio"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type DealStream struct {
	p        peer.ID
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ RetrievalDealStream = (*DealStream)(nil)

func (d *DealStream) ReadDealProposal() (retrievalmarket.DealProposal, error) {
	var ds retrievalmarket.DealProposal

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		log.Warn(err)
		return retrievalmarket.DealProposalUndefined, err
	}
	return ds, nil
}

func (d *DealStream) WriteDealProposal(dp retrievalmarket.DealProposal) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *DealStream) ReadDealResponse() (retrievalmarket.DealResponse, error) {
	var dr retrievalmarket.DealResponse

	if err := dr.UnmarshalCBOR(d.buffered); err != nil {
		return retrievalmarket.DealResponseUndefined, err
	}
	return dr, nil
}

func (d *DealStream) WriteDealResponse(dr retrievalmarket.DealResponse) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *DealStream) ReadDealPayment() (retrievalmarket.DealPayment, error) {
	var ds retrievalmarket.DealPayment

	if err := ds.UnmarshalCBOR(d.rw); err != nil {
		return retrievalmarket.DealPaymentUndefined, err
	}
	return ds, nil
}

func (d *DealStream) WriteDealPayment(dpy retrievalmarket.DealPayment) error {
	return cborutil.WriteCborRPC(d.rw, &dpy)
}

func (d *DealStream) Receiver() peer.ID {
	return d.p
}

func (d *DealStream) Close() error {
	return d.rw.Close()
}
