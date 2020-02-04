package network

import (
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
)

type dealStream struct {
	p  peer.ID
	rw mux.MuxedStream
}

var _ StorageDealStream = (*dealStream)(nil)

func (d *dealStream) ReadDealProposal() (Proposal, error) {
	var ds Proposal

	if err := ds.UnmarshalCBOR(d.rw); err != nil {
		log.Warn(err)
		return ProposalUndefined, err
	}
	return ds, nil
}

func (d *dealStream) WriteDealProposal(dp Proposal) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *dealStream) ReadDealResponse() (SignedResponse, error) {
	var dr SignedResponse

	if err := dr.UnmarshalCBOR(d.rw); err != nil {
		return SignedResponseUndefined, err
	}
	return dr, nil
}

func (d *dealStream) WriteDealResponse(dr SignedResponse) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *dealStream) Close() error {
	return d.rw.Close()
}
