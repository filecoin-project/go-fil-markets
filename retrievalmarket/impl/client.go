package retrievalimpl

import (
	"context"
	"reflect"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/blockio"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/clientstates"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
)

var log = logging.Logger("retrieval")

type client struct {
	network rmnet.RetrievalMarketNetwork
	bs      blockstore.Blockstore
	node    retrievalmarket.RetrievalClientNode
	// The parameters should be replaced by RetrievalClientNode

	nextDealLk sync.RWMutex
	nextDealID retrievalmarket.DealID

	subscribersLk sync.RWMutex
	subscribers   []retrievalmarket.ClientSubscriber
	resolver      retrievalmarket.PeerResolver
}

var _ retrievalmarket.RetrievalClient = &client{}

// NewClient creates a new retrieval client
func NewClient(
	network rmnet.RetrievalMarketNetwork,
	bs blockstore.Blockstore,
	node retrievalmarket.RetrievalClientNode,
	resolver retrievalmarket.PeerResolver) retrievalmarket.RetrievalClient {
	return &client{
		network:  network,
		bs:       bs,
		node:     node,
		resolver: resolver,
	}
}

// V0

func (c *client) FindProviders(payloadCID cid.Cid) []retrievalmarket.RetrievalPeer {
	peers, err := c.resolver.GetPeers(payloadCID)
	if err != nil {
		log.Error(err)
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
func (c *client) Retrieve(ctx context.Context, payloadCID cid.Cid, params retrievalmarket.Params, totalFunds tokenamount.TokenAmount, miner peer.ID, clientWallet address.Address, minerWallet address.Address) retrievalmarket.DealID {
	/* The implementation of this function is just wrapper for the old code which retrieves UnixFS pieces
	-- it will be replaced when we do the V0 implementation of the module */
	c.nextDealLk.Lock()
	c.nextDealID++
	dealID := c.nextDealID
	c.nextDealLk.Unlock()

	dealState := retrievalmarket.ClientDealState{
		DealProposal: retrievalmarket.DealProposal{
			PayloadCID: payloadCID,
			ID:         dealID,
			Params:     params,
		},
		Message:          "starting handle deal",
		TotalFunds:       totalFunds,
		ClientWallet:     clientWallet,
		MinerWallet:      minerWallet,
		TotalReceived:    0,
		CurrentInterval:  params.PaymentInterval,
		BytesPaidFor:     0,
		PaymentRequested: tokenamount.FromInt(0),
		FundsSpent:       tokenamount.FromInt(0),
		Status:           retrievalmarket.DealStatusNew,
		Sender:           miner,
	}

	go c.handleDeal(ctx, dealState)

	return dealID
}

func (c *client) failDeal(dealState *retrievalmarket.ClientDealState, err error) {
	dealState.Message = err.Error()
	dealState.Status = retrievalmarket.DealStatusFailed
	c.notifySubscribers(retrievalmarket.ClientEventError, *dealState)
}

func (c *client) handleDeal(ctx context.Context, dealState retrievalmarket.ClientDealState) {
	c.notifySubscribers(retrievalmarket.ClientEventOpen, dealState)

	s, err := c.network.NewDealStream(dealState.Sender)
	if err != nil {
		c.failDeal(&dealState, err)
		return
	}
	defer s.Close()

	environment := clientDealEnvironment{c.node, blockio.NewSelectorVerifier(cidlink.Link{Cid: dealState.DealProposal.PayloadCID}), c.bs, s}

	for {
		var handler clientstates.ClientHandlerFunc

		switch dealState.Status {
		case retrievalmarket.DealStatusNew:
			handler = clientstates.ProposeDeal
		case retrievalmarket.DealStatusAccepted:
			handler = clientstates.SetupPaymentChannel
		case retrievalmarket.DealStatusPaymentChannelCreated, retrievalmarket.DealStatusOngoing, retrievalmarket.DealStatusUnsealing:
			handler = clientstates.ProcessNextResponse
		case retrievalmarket.DealStatusFundsNeeded, retrievalmarket.DealStatusFundsNeededLastPayment:
			handler = clientstates.ProcessPaymentRequested
		default:
			c.failDeal(&dealState, xerrors.New("unexpected deal state"))
			return
		}
		dealModifier := handler(ctx, &environment, dealState)
		dealModifier(&dealState)
		if retrievalmarket.IsTerminalStatus(dealState.Status) {
			break
		}
		dealState.Message = dealState.Message + " done handling"
		c.notifySubscribers(retrievalmarket.ClientEventProgress, dealState)
	}
	if retrievalmarket.IsTerminalSuccess(dealState.Status) {
		dealState.Message = dealState.Message + "IsTerminalSuccess"
		c.notifySubscribers(retrievalmarket.ClientEventComplete, dealState)
	} else {
		dealState.Message = dealState.Message + "After all that, we failed."
		c.notifySubscribers(retrievalmarket.ClientEventError, dealState)
	}
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

func (c *client) notifySubscribers(evt retrievalmarket.ClientEvent, ds retrievalmarket.ClientDealState) {
	c.subscribersLk.RLock()
	defer c.subscribersLk.RUnlock()
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
func (c *client) AddMoreFunds(id retrievalmarket.DealID, amount tokenamount.TokenAmount) error {
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

type clientDealEnvironment struct {
	node     retrievalmarket.RetrievalClientNode
	verifier blockio.BlockVerifier
	bs       blockstore.Blockstore
	stream   rmnet.RetrievalDealStream
}

func (cde *clientDealEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return cde.node
}

func (cde *clientDealEnvironment) DealStream() rmnet.RetrievalDealStream {
	return cde.stream
}

func (cde *clientDealEnvironment) ConsumeBlock(ctx context.Context, block retrievalmarket.Block) (uint64, bool, error) {
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

	done, err := cde.verifier.Verify(ctx, blk)
	if err != nil {
		log.Warnf("block verify failed: %s", err)
		return 0, false, err
	}

	// TODO: Smarter out, maybe add to filestore automagically
	//  (Also, persist intermediate nodes)
	err = cde.bs.Put(blk)
	if err != nil {
		log.Warnf("block write failed: %s", err)
		return 0, false, err
	}

	return uint64(len(block.Data)), done, nil
}
