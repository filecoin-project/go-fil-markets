package network

import (
	"github.com/filecoin-project/go-fil-components/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-components/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-components/shared/cborutil"
	"github.com/filecoin-project/go-fil-components/shared/tokenamount"
	"github.com/ipfs/go-cid"
	p2pnet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type queryStream struct {
	p peer.ID
	s p2pnet.Stream
}

var _ RetrievalQueryStream = (*queryStream)(nil)

func (qs queryStream) ReadQuery() (retrievalmarket.Query, error) {
	panic("implement me")
}

func (qs queryStream) WriteQuery(q retrievalmarket.Query) error {
	cid, err := cid.Cast(q.PieceCID)
	if err != nil {
		return err
	}

	return cborutil.WriteCborRPC(qs.s, &retrievalimpl.OldQuery{Piece: cid})
}

func (qs queryStream) ReadQueryResponse() (retrievalmarket.QueryResponse, error) {
	var oldResp retrievalimpl.OldQueryResponse
	if err := oldResp.UnmarshalCBOR(qs.s); err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	resp := retrievalmarket.QueryResponse{
		Status:          retrievalmarket.QueryResponseStatus(oldResp.Status),
		Size:            oldResp.Size,
		MinPricePerByte: tokenamount.Div(oldResp.MinPrice, tokenamount.FromInt(oldResp.Size)),
	}
	return resp, nil
}

func (qs queryStream) WriteQueryResponse(retrievalmarket.QueryResponse) error {
	panic("implement me")
}

func (qs queryStream) Close() error {
	return qs.s.Close()
}
