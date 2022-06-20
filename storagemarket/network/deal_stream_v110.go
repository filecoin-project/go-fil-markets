package network

import (
	"bufio"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

type dealStreamv110 struct {
	p        peer.ID
	host     host.Host
	rw       mux.MuxedStream
	buffered *bufio.Reader
}

var _ StorageDealStream = (*dealStreamv110)(nil)

func (d *dealStreamv110) ReadDealProposal() (Proposal, cid.Cid, error) {
	var ds migrations.Proposal1

	if err := ds.UnmarshalCBOR(d.buffered); err != nil {
		err = fmt.Errorf("unmarshalling v110 deal proposal: %w", err)
		log.Warnf(err.Error())
		return ProposalUndefined, cid.Undef, err
	}

	proposalNd, err := cborutil.AsIpld(ds.DealProposal)
	if err != nil {
		err = fmt.Errorf("getting v110 deal proposal as IPLD: %w", err)
		log.Warnf(err.Error())
		return ProposalUndefined, cid.Undef, err
	}

	prop, err := migrations.MigrateClientDealProposal0To1(*ds.DealProposal)
	if err != nil {
		err = fmt.Errorf("migrating v110 deal proposal to current version: %w", err)
		log.Warnf(err.Error())
		return ProposalUndefined, cid.Undef, err
	}
	return Proposal{
		DealProposal:  prop,
		Piece:         ds.Piece,
		FastRetrieval: ds.FastRetrieval,
	}, proposalNd.Cid(), nil
}

func (d *dealStreamv110) WriteDealProposal(dp Proposal) error {
	return cborutil.WriteCborRPC(d.rw, &dp)
}

func (d *dealStreamv110) ReadDealResponse() (SignedResponse, []byte, error) {
	var dr SignedResponse

	if err := dr.UnmarshalCBOR(d.buffered); err != nil {
		return SignedResponseUndefined, nil, err
	}
	origBytes, err := cborutil.Dump(&dr.Response)
	if err != nil {
		return SignedResponseUndefined, nil, err
	}
	return dr, origBytes, nil
}

func (d *dealStreamv110) WriteDealResponse(dr SignedResponse, _ ResigningFunc) error {
	return cborutil.WriteCborRPC(d.rw, &dr)
}

func (d *dealStreamv110) Close() error {
	return d.rw.Close()
}

func (d *dealStreamv110) RemotePeer() peer.ID {
	return d.p
}
