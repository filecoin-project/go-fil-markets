package retrievalimpl

import (
	"context"
	"reflect"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-statemachine/fsm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/blockio"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storedcounter"
)

var log = logging.Logger("retrieval")

type client struct {
	network       rmnet.RetrievalMarketNetwork
	bs            blockstore.Blockstore
	node          retrievalmarket.RetrievalClientNode
	storedCounter *storedcounter.StoredCounter

	subscribersLk  sync.RWMutex
	subscribers    []retrievalmarket.ClientSubscriber
	resolver       retrievalmarket.PeerResolver
	blockVerifiers map[retrievalmarket.DealID]blockio.BlockVerifier
	dealStreams    map[retrievalmarket.DealID]rmnet.RetrievalDealStream
	stateMachines  fsm.Group
}

var _ retrievalmarket.RetrievalClient = &client{}

// ClientDsPrefix is the datastore for the client retrievals key
var ClientDsPrefix = "/retrievals/client"

// NewClient creates a new retrieval client
func NewClient(
	network rmnet.RetrievalMarketNetwork,
	bs blockstore.Blockstore,
	node retrievalmarket.RetrievalClientNode,
	resolver retrievalmarket.PeerResolver,
	ds datastore.Batching,
	storedCounter *storedcounter.StoredCounter,
) (retrievalmarket.RetrievalClient, error) {
	c := &client{
		network:        network,
		bs:             bs,
		node:           node,
		resolver:       resolver,
		storedCounter:  storedCounter,
		dealStreams:    make(map[retrievalmarket.DealID]rmnet.RetrievalDealStream),
		blockVerifiers: make(map[retrievalmarket.DealID]blockio.BlockVerifier),
	}
	stateMachines, err := fsm.New(namespace.Wrap(ds, datastore.NewKey(ClientDsPrefix)), fsm.Parameters{
		Environment:     c,
		StateType:       retrievalmarket.ClientDealState{},
		StateKeyField:   "Status",
		Events:          clientstates.ClientEvents,
		StateEntryFuncs: clientstates.ClientStateEntryFuncs,
		Notifier:        c.notifySubscribers,
	})
	if err != nil {
		return nil, err
	}
	c.stateMachines = stateMachines
	return c, nil
}

// V0

func (c *client) FindProviders(payloadCID cid.Cid) []retrievalmarket.RetrievalPeer {
	peers, err := c.resolver.GetPeers(payloadCID)
	if err != nil {
		log.Errorf("failed to get peers: %s", err)
		return []retrievalmarket.RetrievalPeer{}
	}
	return peers
}

func (c *client) Query(ctx context.Context, p retrievalmarket.RetrievalPeer, payloadCID cid.Cid, params retrievalmarket.QueryParams) (retrievalmarket.QueryResponse, error) {
	s, err := c.network.NewQueryStream(p.ID)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}
	defer s.Close()

	err = s.WriteQuery(retrievalmarket.Query{
		PayloadCID: payloadCID,
	})
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return s.ReadQueryResponse()
}

// Retrieve begins the process of requesting the data referred to by payloadCID, after a deal is accepted
func (c *client) Retrieve(ctx context.Context, payloadCID cid.Cid, params retrievalmarket.Params, totalFunds abi.TokenAmount, miner peer.ID, clientWallet address.Address, minerWallet address.Address) (retrievalmarket.DealID, error) {
	next, err := c.storedCounter.Next()
	if err != nil {
		return 0, err
	}
	dealID := retrievalmarket.DealID(next)

	dealState := retrievalmarket.ClientDealState{
		DealProposal: retrievalmarket.DealProposal{
			PayloadCID: payloadCID,
			ID:         dealID,
			Params:     params,
		},
		TotalFunds:       totalFunds,
		ClientWallet:     clientWallet,
		MinerWallet:      minerWallet,
		TotalReceived:    0,
		CurrentInterval:  params.PaymentInterval,
		BytesPaidFor:     0,
		PaymentRequested: abi.NewTokenAmount(0),
		FundsSpent:       abi.NewTokenAmount(0),
		Status:           retrievalmarket.DealStatusNew,
		Sender:           miner,
	}

	// start the deal processing
	err = c.stateMachines.Begin(dealState.ID, &dealState)
	if err != nil {
		return 0, err
	}

	// open stream
	s, err := c.network.NewDealStream(dealState.Sender)
	if err != nil {
		return 0, err
	}

	c.dealStreams[dealID] = s

	sel, err := allSelector()
	if err != nil {
		return 0, err
	}
	c.blockVerifiers[dealID] = blockio.NewSelectorVerifier(cidlink.Link{Cid: dealState.DealProposal.PayloadCID}, sel)

	err = c.stateMachines.Send(dealState.ID, retrievalmarket.ClientEventOpen)
	if err != nil {
		s.Close()
		return 0, err
	}

	return dealID, nil
}

// unsubscribeAt returns a function that removes an item from the subscribers list by comparing
// their reflect.ValueOf before pulling the item out of the slice.  Does not preserve order.
// Subsequent, repeated calls to the func with the same Subscriber are a no-op.
func (c *client) unsubscribeAt(sub retrievalmarket.ClientSubscriber) retrievalmarket.Unsubscribe {
	return func() {
		c.subscribersLk.Lock()
		defer c.subscribersLk.Unlock()
		curLen := len(c.subscribers)
		for i, el := range c.subscribers {
			if reflect.ValueOf(sub) == reflect.ValueOf(el) {
				c.subscribers[i] = c.subscribers[curLen-1]
				c.subscribers = c.subscribers[:curLen-1]
				return
			}
		}
	}
}

func (c *client) notifySubscribers(eventName fsm.EventName, state fsm.StateType) {
	c.subscribersLk.RLock()
	defer c.subscribersLk.RUnlock()
	evt := eventName.(retrievalmarket.ClientEvent)
	ds := state.(retrievalmarket.ClientDealState)
	for _, cb := range c.subscribers {
		cb(evt, ds)
	}
}

func (c *client) SubscribeToEvents(subscriber retrievalmarket.ClientSubscriber) retrievalmarket.Unsubscribe {
	c.subscribersLk.Lock()
	c.subscribers = append(c.subscribers, subscriber)
	c.subscribersLk.Unlock()

	return c.unsubscribeAt(subscriber)
}

// V1
func (c *client) AddMoreFunds(id retrievalmarket.DealID, amount abi.TokenAmount) error {
	panic("not implemented")
}

func (c *client) CancelDeal(id retrievalmarket.DealID) error {
	panic("not implemented")
}

func (c *client) RetrievalStatus(id retrievalmarket.DealID) {
	panic("not implemented")
}

func (c *client) ListDeals() map[retrievalmarket.DealID]retrievalmarket.ClientDealState {
	panic("not implemented")
}

func (c *client) Node() retrievalmarket.RetrievalClientNode {
	return c.node
}

func (c *client) DealStream(dealID retrievalmarket.DealID) rmnet.RetrievalDealStream {
	return c.dealStreams[dealID]
}

func (c *client) ConsumeBlock(ctx context.Context, dealID retrievalmarket.DealID, block retrievalmarket.Block) (uint64, bool, error) {
	prefix, err := cid.PrefixFromBytes(block.Prefix)
	if err != nil {
		return 0, false, err
	}

	cid, err := prefix.Sum(block.Data)
	if err != nil {
		return 0, false, err
	}

	blk, err := blocks.NewBlockWithCid(block.Data, cid)
	if err != nil {
		return 0, false, err
	}

	verifier, ok := c.blockVerifiers[dealID]
	if !ok {
		return 0, false, xerrors.New("no block verifier found")
	}

	done, err := verifier.Verify(ctx, blk)
	if err != nil {
		log.Warnf("block verify failed: %s", err)
		return 0, false, err
	}

	// TODO: Smarter out, maybe add to filestore automagically
	//  (Also, persist intermediate nodes)
	err = c.bs.Put(blk)
	if err != nil {
		log.Warnf("block write failed: %s", err)
		return 0, false, err
	}

	return uint64(len(block.Data)), done, nil
}
