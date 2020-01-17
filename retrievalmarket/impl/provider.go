package retrievalimpl

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-blockservice"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
)

// UnixfsReader is a unixfsfile that can read block by block
type UnixfsReader interface {
	files.File

	// ReadBlock reads data from a single unixfs block. Data is nil
	// for intermediate nodes
	ReadBlock(context.Context) (data []byte, offset uint64, nd ipld.Node, err error)
}

type provider struct {

	// TODO: Replace with RetrievalProviderNode for
	// https://github.com/filecoin-project/go-retrieval-market-project/issues/4
	node                    retrievalmarket.RetrievalProviderNode
	network                 rmnet.RetrievalMarketNetwork
	paymentInterval         uint64
	paymentIntervalIncrease uint64
	paymentAddress          address.Address
	pricePerByte            tokenamount.TokenAmount
	subscribers             []retrievalmarket.ProviderSubscriber
	subscribersLk           sync.RWMutex
}

var _ retrievalmarket.RetrievalProvider = &provider{}

// NewProvider returns a new retrieval provider
func NewProvider(paymentAddress address.Address, node retrievalmarket.RetrievalProviderNode, network rmnet.RetrievalMarketNetwork) retrievalmarket.RetrievalProvider {
	return &provider{
		node:           node,
		network:        network,
		paymentAddress: paymentAddress,
		pricePerByte:   tokenamount.FromInt(2), // TODO: allow setting
	}
}

// Start begins listening for deals on the given host
func (p *provider) Start() error {
	return p.network.SetDelegate(p)
}

// V0
// SetPricePerByte sets the price per byte a miner charges for retrievals
func (p *provider) SetPricePerByte(price tokenamount.TokenAmount) {
	p.pricePerByte = price
}

// SetPaymentInterval sets the maximum number of bytes a a provider will send before
// requesting further payment, and the rate at which that value increases
// TODO: Implement for https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) SetPaymentInterval(paymentInterval uint64, paymentIntervalIncrease uint64) {
	p.paymentInterval = paymentInterval
	p.paymentIntervalIncrease = paymentIntervalIncrease
}

// unsubscribeAt returns a function that removes an item from the subscribers list by comparing
// their reflect.ValueOf before pulling the item out of the slice.  Does not preserve order.
// Subsequent, repeated calls to the func with the same Subscriber are a no-op.
func (p *provider) unsubscribeAt(sub retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	return func() {
		p.subscribersLk.Lock()
		defer p.subscribersLk.Unlock()
		curLen := len(p.subscribers)
		for i, el := range p.subscribers {
			if reflect.ValueOf(sub) == reflect.ValueOf(el) {
				p.subscribers[i] = p.subscribers[curLen-1]
				p.subscribers = p.subscribers[:curLen-1]
				return
			}
		}
	}
}

func (p *provider) notifySubscribers(evt retrievalmarket.ProviderEvent, ds retrievalmarket.ProviderDealState) {
	p.subscribersLk.RLock()
	defer p.subscribersLk.RUnlock()
	for _, cb := range p.subscribers {
		cb(evt, ds)
	}
}

// SubscribeToEvents listens for events that happen related to client retrievals
// TODO: Implement updates as part of https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) SubscribeToEvents(subscriber retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	p.subscribersLk.Lock()
	p.subscribers = append(p.subscribers, subscriber)
	p.subscribersLk.Unlock()

	return p.unsubscribeAt(subscriber)
}

// V1
func (p *provider) SetPricePerUnseal(price tokenamount.TokenAmount) {
	panic("not implemented")
}

func (p *provider) ListDeals() map[retrievalmarket.ProviderDealID]retrievalmarket.ProviderDealState {
	panic("not implemented")
}

// TODO: Update for https://github.com/filecoin-project/go-retrieval-market-project/issues/8
func (p *provider) HandleQueryStream(stream rmnet.RetrievalQueryStream) {
	defer stream.Close()
	query, err := stream.ReadQuery()
	if err != nil {
		return
	}

	answer := retrievalmarket.QueryResponse{
		Status:                     retrievalmarket.QueryResponseUnavailable,
		PaymentAddress:             p.paymentAddress,
		MinPricePerByte:            p.pricePerByte,
		MaxPaymentInterval:         p.paymentInterval,
		MaxPaymentIntervalIncrease: p.paymentIntervalIncrease,
	}

	size, err := p.node.GetPieceSize(query.PieceCID)

	if err == nil {
		answer.Status = retrievalmarket.QueryResponseAvailable
		// TODO: get price, look for already unsealed ref to reduce work
		answer.Size = uint64(size) // TODO: verify on intermediate
	}

	if err != nil && err != retrievalmarket.ErrNotFound {
		log.Errorf("Retrieval query: GetRefs: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		answer.Message = err.Error()
	}

	if err := stream.WriteQueryResponse(answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}

func (p *provider) failDeal(dealState *retrievalmarket.ProviderDealState, err error) {
	dealState.Message = err.Error()
	dealState.Status = retrievalmarket.DealStatusFailed
	p.notifySubscribers(retrievalmarket.ProviderEventError, *dealState)
}

// TODO: Update for https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) HandleDealStream(stream rmnet.RetrievalDealStream) {
	defer stream.Close()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	dealState := retrievalmarket.ProviderDealState{
		Status:        retrievalmarket.DealStatusNew,
		TotalSent:     0,
		FundsReceived: tokenamount.FromInt(0),
	}
	p.notifySubscribers(retrievalmarket.ProviderEventOpen, dealState)

	bstore := p.node.SealedBlockstore(func() error {
		return nil // TODO: approve unsealing based on amount paid
	})

	ds := merkledag.NewDAGService(blockservice.New(bstore, nil))

	environment := providerDealEnvironment{p.node, 0, 0, nil, p.pricePerByte, p.paymentInterval, p.paymentIntervalIncrease, stream}

	for {
		var handler providerstates.ProviderHandlerFunc

		switch dealState.Status {
		case retrievalmarket.DealStatusNew:
			handler = providerstates.ReceiveDeal
		case retrievalmarket.DealStatusAccepted, retrievalmarket.DealStatusOngoing, retrievalmarket.DealStatusUnsealing:
			handler = providerstates.SendBlocks
		case retrievalmarket.DealStatusFundsNeeded, retrievalmarket.DealStatusFundsNeededLastPayment:
			handler = providerstates.ProcessPayment
		default:
			p.failDeal(&dealState, errors.New("unexpected deal state"))
			return
		}
		dealModifier := handler(ctx, &environment, dealState)
		dealModifier(&dealState)
		if retrievalmarket.IsTerminalStatus(dealState.Status) {
			break
		}
		if environment.ufsr == nil {
			rootNd, err := ds.Get(context.TODO(), dealState.PayloadCID)
			if err != nil {
				p.failDeal(&dealState, err)
				return
			}

			fsr, err := unixfile.NewUnixfsFile(context.TODO(), ds, rootNd)
			if err != nil {
				p.failDeal(&dealState, err)
				return
			}

			var ok bool
			environment.ufsr, ok = fsr.(UnixfsReader)
			if !ok {
				p.failDeal(&dealState, xerrors.Errorf("file %s didn't implement UnixfsReader", dealState.PayloadCID))
				return
			}
			size, err := fsr.Size()
			if err != nil {
				p.failDeal(&dealState, xerrors.Errorf("file %s didn't implement UnixfsReader", dealState.PayloadCID))
				return
			}
			environment.size = uint64(size)
		}
		p.notifySubscribers(retrievalmarket.ProviderEventProgress, dealState)
	}
	if retrievalmarket.IsTerminalSuccess(dealState.Status) {
		p.notifySubscribers(retrievalmarket.ProviderEventComplete, dealState)
	} else {
		p.notifySubscribers(retrievalmarket.ProviderEventError, dealState)
	}
}

type providerDealEnvironment struct {
	node                       retrievalmarket.RetrievalProviderNode
	read                       uint64
	size                       uint64
	ufsr                       UnixfsReader
	minPricePerByte            tokenamount.TokenAmount
	maxPaymentInterval         uint64
	maxPaymentIntervalIncrease uint64
	stream                     rmnet.RetrievalDealStream
}

func (pde *providerDealEnvironment) Node() retrievalmarket.RetrievalProviderNode {
	return pde.node
}

func (pde *providerDealEnvironment) DealStream() rmnet.RetrievalDealStream {
	return pde.stream
}

func (pde *providerDealEnvironment) CheckDealParams(pricePerByte tokenamount.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64) error {
	if pricePerByte.LessThan(pde.minPricePerByte) {
		return errors.New("Price per byte too low")
	}
	if paymentInterval > pde.maxPaymentInterval {
		return errors.New("Payment interval too large")
	}
	if paymentIntervalIncrease > pde.maxPaymentIntervalIncrease {
		return errors.New("Payment interval increase too large")
	}
	return nil
}

func (pde *providerDealEnvironment) NextBlock(ctx context.Context) (retrievalmarket.Block, bool, error) {
	if pde.ufsr == nil {
		return retrievalmarket.Block{}, false, errors.New("Could not read block")
	}
	data, _, nd, err := pde.ufsr.ReadBlock(ctx)
	if err != nil {
		return retrievalmarket.Block{}, false, err
	}
	block := retrievalmarket.Block{
		Prefix: nd.Cid().Prefix().Bytes(),
		Data:   nd.RawData(),
	}
	pde.read += uint64(len(data))
	done := pde.read >= pde.size
	return block, done, nil
}
