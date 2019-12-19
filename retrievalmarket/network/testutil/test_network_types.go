package testutil

import (
	"errors"

	"github.com/libp2p/go-libp2p-core/host"
	p2pnet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	rm "github.com/filecoin-project/go-fil-components/retrievalmarket"
	rmnet "github.com/filecoin-project/go-fil-components/retrievalmarket/network"
)

type TestRetrievalQueryStream struct{
	p peer.ID
	s p2pnet.Stream
	failRead, failWrite bool
}

func (trqs TestRetrievalQueryStream) NewTestRetrievalQueryStream(p peer.ID, failRead, failWrite bool) {
}

func (trqs TestRetrievalQueryStream)ReadQuery() (rm.Query, error){
	if trqs.failRead {
		return rm.Query{}, errors.New("fail ReadQuery")
	}

	return rm.Query{}, nil
}
func (trqs TestRetrievalQueryStream)WriteQuery(newQuery rm.Query) error{
	if trqs.failWrite {
		return errors.New("fail WriteQuery")
	}
	return nil
}
func (trqs TestRetrievalQueryStream)ReadQueryResponse() (rm.QueryResponse, error){
	if trqs.failRead {
		return rm.QueryResponse{}, errors.New("fail ReadQueryResponse")
	}



	return rm.QueryResponse{}, nil
}
func (trqs TestRetrievalQueryStream)WriteQueryResponse(newResp rm.QueryResponse) error{
	if trqs.failWrite {
		return errors.New("fail WriteQueryResponse")
	}
	return nil
}

func (trqs TestRetrievalQueryStream) Close() error { return nil }


//type TestRetrievalDealStream struct{
//	dprop rm.DealProposal
//	dresp rm.DealResponse
//	dpaym rm.DealPayment
//
//	failRead, failWrite bool
//}
//
//func NewTestRetrievalDealStream(dprop rm.DealProposal, dresp rm.DealResponse, dpaym rm.DealPayment, fr, fw bool) *TestRetrievalDealStream {
//	return &TestRetrievalDealStream{ dprop, dresp, dpaym, fr, fw }
//}
//
//func (trds TestRetrievalDealStream)ReadDealProposal() (rm.DealProposal, error){
//	if trds.failRead {
//		return rm.DealProposal{}, errors.New("fail ReadDealProposal")
//	}
//	return trds.dprop, nil
//}
//func (trds TestRetrievalDealStream)WriteDealProposal(rm.DealProposal) error{
//	if trds.failWrite {
//		return errors.New("fail WriteDealProposal")
//	}
//	return nil
//}
//func (trds TestRetrievalDealStream)ReadDealResponse() (rm.DealResponse, error){
//	if trds.failRead {
//		return rm.DealResponse{}, errors.New("fail ReadDealResponse")
//	}
//	return trds.dresp, nil
//}
//func (trds TestRetrievalDealStream)WriteDealResponse(rm.DealResponse) error{
//	if trds.failWrite {
//		return errors.New("fail WriteDealResponse")
//	}
//	return nil
//}
//
//func (trds TestRetrievalDealStream)ReadDealPayment() (rm.DealPayment, error){
//	if trds.failRead {
//		return rm.DealPayment{}, errors.New("fail ReadDealPayment")
//	}
//	return trds.dpaym, nil
//}
//func (trds TestRetrievalDealStream)WriteDealPayment(rm.DealPayment) error{
//	if trds.failWrite {
//		return errors.New("fail WriteDealPayment")
//	}
//	return nil
//}
//func (trqs TestRetrievalDealStream) Close() error { return nil }
//
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

type TestRetrievalMarketNetwork struct{
	netHost host.Host
	receiver rmnet.RetrievalReceiver
	peers []peer.ID
}

func NewTestRetrievalMarketNetwork(netHost host.Host, peers []peer.ID) *TestRetrievalMarketNetwork {
	return &TestRetrievalMarketNetwork{ netHost:netHost, peers:peers}
}

func (trmn TestRetrievalMarketNetwork)NewQueryStream(id peer.ID) (rmnet.RetrievalQueryStream, error){
	return TestRetrievalQueryStream{}, nil
}
func (trmn TestRetrievalMarketNetwork)NewDealStream(id peer.ID) (rmnet.RetrievalDealStream, error){
	return nil, nil
}
func (trmn TestRetrievalMarketNetwork)SetDelegate(r rmnet.RetrievalReceiver) error {
	trmn.receiver = r
	return nil
}
