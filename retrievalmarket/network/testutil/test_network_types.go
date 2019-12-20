package testutil

import (
	"context"
	"errors"
	"github.com/filecoin-project/go-address"
	rm "github.com/filecoin-project/go-fil-components/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-components/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-components/shared/tokenamount"
	"github.com/filecoin-project/go-fil-components/shared/types"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

type TestRetrievalQueryStream struct {
	p          peer.ID
	reader     func() (rm.Query, error)
	respReader func() (rm.QueryResponse, error)
	respWriter func(rm.QueryResponse) error
	writer     func(rm.Query) error
}

type TestQueryStreamParams struct {
	PeerID     peer.ID
	Reader     func() (rm.Query, error)
	RespReader func() (rm.QueryResponse, error)
	RespWriter func(rm.QueryResponse) error
	Writer     func(rm.Query) error
}

func NewTestRetrievalQueryStream(params TestQueryStreamParams) *TestRetrievalQueryStream {
	stream := TestRetrievalQueryStream{
		p:          params.PeerID,
		reader:     TrivialQueryReader,
		respReader: TrivialQueryResponseReader,
		respWriter: TrivialQueryResponseWriter,
		writer:     TrivialQueryWriter,
	}
	if params.Reader != nil {
		stream.reader = params.Reader
	}
	if params.Writer != nil {
		stream.writer = params.Writer
	}
	if params.RespReader != nil {
		stream.respReader = params.RespReader
	}
	if params.RespWriter != nil {
		stream.respWriter = params.RespWriter
	}
	return &stream
}

func (trqs *TestRetrievalQueryStream) ReadQuery() (rm.Query, error) {
	return trqs.reader()
}
func (trqs *TestRetrievalQueryStream) WriteQuery(newQuery rm.Query) error {
	return trqs.writer(newQuery)
}
func (trqs *TestRetrievalQueryStream) ReadQueryResponse() (rm.QueryResponse, error) {
	return trqs.respReader()
}
func (trqs *TestRetrievalQueryStream) WriteQueryResponse(newResp rm.QueryResponse) error {
	return trqs.respWriter(newResp)
}

func (trqs *TestRetrievalQueryStream) Close() error { return nil }

type TestRetrievalDealStream struct {
	failRead, failWrite bool
}

func (trds *TestRetrievalDealStream) ReadDealProposal() (rm.DealProposal, error) {
	if trds.failRead {
		return rm.DealProposal{}, errors.New("fail ReadDealProposal")
	}
	return rm.DealProposal{}, nil
}
func (trds *TestRetrievalDealStream) WriteDealProposal(rm.DealProposal) error {
	if trds.failWrite {
		return errors.New("fail WriteDealProposal")
	}
	return nil
}
func (trds *TestRetrievalDealStream) ReadDealResponse() (rm.DealResponse, error) {
	if trds.failRead {
		return rm.DealResponse{}, errors.New("fail ReadDealResponse")
	}
	return rm.DealResponse{}, nil
}
func (trds *TestRetrievalDealStream) WriteDealResponse(rm.DealResponse) error {
	if trds.failWrite {
		return errors.New("fail WriteDealResponse")
	}
	return nil
}

func (trds *TestRetrievalDealStream) ReadDealPayment() (rm.DealPayment, error) {
	if trds.failRead {
		return rm.DealPayment{}, errors.New("fail ReadDealPayment")
	}
	return rm.DealPayment{}, nil
}
func (trds *TestRetrievalDealStream) WriteDealPayment(rm.DealPayment) error {
	if trds.failWrite {
		return errors.New("fail WriteDealPayment")
	}
	return nil
}
func (trqs TestRetrievalDealStream) Close() error { return nil }

//
//type TestRetrievalReceiver struct{
//	queryStreamHandler func(stream rmnet.RetrievalQueryStream)
//	retrievalDealHandler func(stream rmnet.RetrievalDealStream)
//}
//
//func NewTestRetrievalReceiver(	qsh func(stream rmnet.RetrievalQueryStream),
//								rdh func(stream rmnet.RetrievalDealStream)) *TestRetrievalReceiver {
//	return &TestRetrievalReceiver{ queryStreamHandler: qsh,  retrievalDealHandler: rdh}
//}
//
//func (trr TestRetrievalReceiver)HandleQueryStream(stream rmnet.RetrievalQueryStream){
//	if trr.queryStreamHandler != nil {
//		trr.queryStreamHandler(stream)
//	}
//}
//
//func (trr TestRetrievalReceiver)HandleDealStream(stream rmnet.RetrievalDealStream) {
//	if trr.queryStreamHandler != nil {
//		trr.retrievalDealHandler(stream)
//	}
//}

type TestRetrievalMarketNetwork struct {
	netHost  host.Host
	receiver rmnet.RetrievalReceiver
	peers    []peer.ID

	dsbuilder func(peer.ID) (rmnet.RetrievalDealStream, error)

	qsbuilder   func(peer.ID) (rmnet.RetrievalQueryStream, error)
	qrespReader func() (rm.QueryResponse, error)
	qwriter     func(rm.Query) error
}

type TestNetworkParams struct {
	Host  host.Host
	Peers []peer.ID

	DealStreamBuilder func(peer.ID) (rmnet.RetrievalDealStream, error)

	QueryStreamBuilder  func(peer.ID) (rmnet.RetrievalQueryStream, error)
	QueryResponseReader func() (rm.QueryResponse, error)
	QueryWriter         func(rm.Query) error

	Receiver rmnet.RetrievalReceiver
}

func NewTestRetrievalMarketNetwork(params TestNetworkParams) *TestRetrievalMarketNetwork {
	trmn := TestRetrievalMarketNetwork{
		netHost: params.Host,
		peers:   params.Peers,

		dsbuilder: TrivialNewDealStream,
		qsbuilder: TrivialNewQueryStream,

		receiver: params.Receiver,
	}
	if params.DealStreamBuilder != nil {
		trmn.dsbuilder = params.DealStreamBuilder
	}
	if params.QueryStreamBuilder != nil {
		trmn.qsbuilder = params.QueryStreamBuilder
	}
	return &trmn
}

// NewQueryStream returns a query stream.
// Note this always returns the same stream.  This is fine for testing for now.
func (trmn *TestRetrievalMarketNetwork) NewQueryStream(id peer.ID) (rmnet.RetrievalQueryStream, error) {
	return trmn.qsbuilder(id)
}
func (trmn *TestRetrievalMarketNetwork) NewDealStream(id peer.ID) (rmnet.RetrievalDealStream, error) {
	return nil, nil
}
func (trmn *TestRetrievalMarketNetwork) SetDelegate(r rmnet.RetrievalReceiver) error {
	trmn.receiver = r
	return nil
}

type TestRetrievalNode struct {
	NodeAddr, PmtAddr address.Address
}

var _ rm.RetrievalClientNode = (*TestRetrievalNode)(nil)

func (t *TestRetrievalNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable tokenamount.TokenAmount) (address.Address, error) {
	return address.Address{}, nil
}

func (t *TestRetrievalNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	return 0, nil
}

func (t *TestRetrievalNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error) {
	return nil, nil
}

// Some convenience builders
// FailNewQueryStream always fails
func FailNewQueryStream(peer.ID) (rmnet.RetrievalQueryStream, error) {
	return nil, errors.New("new query stream failed")
}

// FailNewDealStream always fails
func FailNewDealStream(peer.ID) (rmnet.RetrievalDealStream, error) {
	return nil, errors.New("new deal stream failed")
}
func FailQueryWriter(rm.Query) error {
	return errors.New("write query failed")
}

// TrivialNewQueryStream succeeds trivially, returning an empty query stream.
func TrivialNewQueryStream(p peer.ID) (rmnet.RetrievalQueryStream, error) {
	return NewTestRetrievalQueryStream(TestQueryStreamParams{PeerID: p}), nil
}

// TrivialNewDealStream succeeds trivially, returning an empty deal stream.
func TrivialNewDealStream(peer.ID) (rmnet.RetrievalDealStream, error) {
	return &TestRetrievalDealStream{}, errors.New("new deal stream failed")
}

// TrivialQueryReader succeeds trivially, returning an empty query.
func TrivialQueryReader() (rm.Query, error) {
	return rm.Query{}, nil
}

// TrivialQueryResponseReader succeeds trivially, returning an empty query response.
func TrivialQueryResponseReader() (rm.QueryResponse, error) {
	return rm.QueryResponse{}, nil
}

// TrivialQueryWriter succeeds trivially, returning no error.
func TrivialQueryWriter(rm.Query) error {
	return nil
}

// TrivialQueryResponseWriter succeeds trivially, returning no error.
func TrivialQueryResponseWriter(rm.QueryResponse) error {
	return nil
}
