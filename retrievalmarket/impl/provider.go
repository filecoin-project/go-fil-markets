package retrievalimpl

import (
	"context"
	"errors"
	"sync"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
)

// RetrievalProviderOption is a function that configures a retrieval provider
type RetrievalProviderOption func(p *Provider)

// DealDecider is a function that makes a decision about whether to accept a deal
type DealDecider func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error)

// Provider is the production implementation of the RetrievalProvider interface
type Provider struct {
	multiStore       *multistore.MultiStore
	dataTransfer     datatransfer.Manager
	node             retrievalmarket.RetrievalProviderNode
	network          rmnet.RetrievalMarketNetwork
	requestValidator *requestvalidation.ProviderRequestValidator
	revalidator      *requestvalidation.ProviderRevalidator
	minerAddress     address.Address
	pieceStore       piecestore.PieceStore
	subscribers      *pubsub.PubSub
	stateMachines    fsm.Group
	dealDecider      DealDecider

	askLk sync.Mutex
	ask   *retrievalmarket.Ask
}

type internalProviderEvent struct {
	evt   retrievalmarket.ProviderEvent
	state retrievalmarket.ProviderDealState
}

func providerDispatcher(evt pubsub.Event, subscriberFn pubsub.SubscriberFn) error {
	ie, ok := evt.(internalProviderEvent)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb, ok := subscriberFn.(retrievalmarket.ProviderSubscriber)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb(ie.evt, ie.state)
	return nil
}

var _ retrievalmarket.RetrievalProvider = new(Provider)

// DefaultPricePerByte is the charge per byte retrieved if the miner does
// not specifically set it
var DefaultPricePerByte = abi.NewTokenAmount(2)

// DefaultPaymentInterval is the baseline interval, set to 1Mb
// if the miner does not explicitly set it otherwise
var DefaultPaymentInterval = uint64(1 << 20)

// DefaultPaymentIntervalIncrease is the amount interval increases on each payment,
// set to to 1Mb if the miner does not explicitly set it otherwise
var DefaultPaymentIntervalIncrease = uint64(1 << 20)

// DealDeciderOpt sets a custom protocol
func DealDeciderOpt(dd DealDecider) RetrievalProviderOption {
	return func(provider *Provider) {
		provider.dealDecider = dd
	}
}

// NewProvider returns a new retrieval Provider
func NewProvider(minerAddress address.Address,
	node retrievalmarket.RetrievalProviderNode,
	network rmnet.RetrievalMarketNetwork,
	pieceStore piecestore.PieceStore,
	multiStore *multistore.MultiStore,
	dataTransfer datatransfer.Manager,
	ds datastore.Batching,
	opts ...RetrievalProviderOption,
) (retrievalmarket.RetrievalProvider, error) {

	p := &Provider{
		multiStore:   multiStore,
		dataTransfer: dataTransfer,
		node:         node,
		network:      network,
		minerAddress: minerAddress,
		pieceStore:   pieceStore,
		subscribers:  pubsub.New(providerDispatcher),
		ask: &retrievalmarket.Ask{
			PricePerByte:            DefaultPricePerByte,
			PaymentInterval:         DefaultPaymentInterval,
			PaymentIntervalIncrease: DefaultPaymentIntervalIncrease,
			UnsealPrice:             abi.NewTokenAmount(0),
		},
	}
	statemachines, err := fsm.New(ds, fsm.Parameters{
		Environment:     &providerDealEnvironment{p},
		StateType:       retrievalmarket.ProviderDealState{},
		StateKeyField:   "Status",
		Events:          providerstates.ProviderEvents,
		StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
		Notifier:        p.notifySubscribers,
	})
	if err != nil {
		return nil, err
	}
	p.Configure(opts...)
	p.stateMachines = statemachines
	p.requestValidator = requestvalidation.NewProviderRequestValidator(&providerValidationEnvironment{p})
	err = p.dataTransfer.RegisterVoucherType(&retrievalmarket.DealProposal{}, p.requestValidator)
	if err != nil {
		return nil, err
	}
	p.revalidator = requestvalidation.NewProviderRevalidator(&providerRevalidatorEnvironment{p})
	err = p.dataTransfer.RegisterRevalidator(&retrievalmarket.DealPayment{}, p.revalidator)
	if err != nil {
		return nil, err
	}
	err = p.dataTransfer.RegisterVoucherResultType(&retrievalmarket.DealResponse{})
	if err != nil {
		return nil, err
	}
	dataTransfer.SubscribeToEvents(dtutils.ProviderDataTransferSubscriber(p.stateMachines))
	err = p.dataTransfer.RegisterTransportConfigurer(&retrievalmarket.DealProposal{},
		dtutils.TransportConfigurer(network.ID(), &providerStoreGetter{p}))
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Stop stops handling incoming requests.
func (p *Provider) Stop() error {
	return p.network.StopHandlingRequests()
}

// Start begins listening for deals on the given host.
// Start must be called in order to accept incoming deals.
func (p *Provider) Start() error {
	return p.network.SetDelegate(p)
}

func (p *Provider) notifySubscribers(eventName fsm.EventName, state fsm.StateType) {
	evt := eventName.(retrievalmarket.ProviderEvent)
	ds := state.(retrievalmarket.ProviderDealState)
	_ = p.subscribers.Publish(internalProviderEvent{evt, ds})
}

// SubscribeToEvents listens for events that happen related to client retrievals
func (p *Provider) SubscribeToEvents(subscriber retrievalmarket.ProviderSubscriber) retrievalmarket.Unsubscribe {
	return retrievalmarket.Unsubscribe(p.subscribers.Subscribe(subscriber))
}

func (p *Provider) GetAsk() *retrievalmarket.Ask {
	p.askLk.Lock()
	defer p.askLk.Unlock()
	a := *p.ask
	return &a
}

func (p *Provider) SetAsk(ask *retrievalmarket.Ask) {
	p.askLk.Lock()
	defer p.askLk.Unlock()
	p.ask = ask
}

// ListDeals lists in all known retrieval deals
func (p *Provider) ListDeals() map[retrievalmarket.ProviderDealIdentifier]retrievalmarket.ProviderDealState {
	var deals []retrievalmarket.ProviderDealState
	_ = p.stateMachines.List(&deals)
	dealMap := make(map[retrievalmarket.ProviderDealIdentifier]retrievalmarket.ProviderDealState)
	for _, deal := range deals {
		dealMap[retrievalmarket.ProviderDealIdentifier{Receiver: deal.Receiver, DealID: deal.ID}] = deal
	}
	return dealMap
}

/*
HandleQueryStream is called by the network implementation whenever a new message is received on the query protocol

A Provider handling a retrieval `Query` does the following:

1. Get the node's chain head in order to get its miner worker address.

2. Look in its piece store for determine if it can serve the given payload CID.

3. Combine these results with its existing parameters for retrieval deals to construct a `retrievalmarket.QueryResponse` struct.

4.0 Writes this response to the `Query` stream.

The connection is kept open only as long as the query-response exchange.
*/
func (p *Provider) HandleQueryStream(stream rmnet.RetrievalQueryStream) {
	defer stream.Close()
	query, err := stream.ReadQuery()
	if err != nil {
		return
	}

	ask := p.GetAsk()

	answer := retrievalmarket.QueryResponse{
		Status:                     retrievalmarket.QueryResponseUnavailable,
		PieceCIDFound:              retrievalmarket.QueryItemUnavailable,
		MinPricePerByte:            ask.PricePerByte,
		MaxPaymentInterval:         ask.PaymentInterval,
		MaxPaymentIntervalIncrease: ask.PaymentIntervalIncrease,
		UnsealPrice:                ask.UnsealPrice,
	}

	ctx := context.TODO()

	tok, _, err := p.node.GetChainHead(ctx)
	if err != nil {
		log.Errorf("Retrieval query: GetChainHead: %s", err)
		return
	}

	paymentAddress, err := p.node.GetMinerWorkerAddress(ctx, p.minerAddress, tok)
	if err != nil {
		log.Errorf("Retrieval query: Lookup Payment Address: %s", err)
		answer.Status = retrievalmarket.QueryResponseError
		answer.Message = err.Error()
	} else {
		answer.PaymentAddress = paymentAddress

		pieceCID := cid.Undef
		if query.PieceCID != nil {
			pieceCID = *query.PieceCID
		}
		pieceInfo, err := getPieceInfoFromCid(p.pieceStore, query.PayloadCID, pieceCID)

		if err == nil && len(pieceInfo.Deals) > 0 {
			answer.Status = retrievalmarket.QueryResponseAvailable
			// TODO: get price, look for already unsealed ref to reduce work
			answer.Size = uint64(pieceInfo.Deals[0].Length) // TODO: verify on intermediate
			answer.PieceCIDFound = retrievalmarket.QueryItemAvailable
		}

		if err != nil && !xerrors.Is(err, retrievalmarket.ErrNotFound) {
			log.Errorf("Retrieval query: GetRefs: %s", err)
			answer.Status = retrievalmarket.QueryResponseError
			answer.Message = err.Error()
		}

	}
	if err := stream.WriteQueryResponse(answer); err != nil {
		log.Errorf("Retrieval query: WriteCborRPC: %s", err)
		return
	}
}

// Configure reconfigures a provider after initialization
func (p *Provider) Configure(opts ...RetrievalProviderOption) {
	for _, opt := range opts {
		opt(p)
	}
}

// ProviderFSMParameterSpec is a valid set of parameters for a provider FSM - used in doc generation
var ProviderFSMParameterSpec = fsm.Parameters{
	Environment:     &providerDealEnvironment{},
	StateType:       retrievalmarket.ProviderDealState{},
	StateKeyField:   "Status",
	Events:          providerstates.ProviderEvents,
	StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
}
