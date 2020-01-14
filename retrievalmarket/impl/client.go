package retrievalimpl

import (
	"context"
	"reflect"
	"sync"

	"github.com/filecoin-project/go-address"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
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

// TODO: Implement for retrieval provider V0 epic
// https://github.com/filecoin-project/go-retrieval-market-project/issues/12
func (c *client) FindProviders(pieceCID []byte) []retrievalmarket.RetrievalPeer {
	peers, err := c.resolver.GetPeers(pieceCID)
	if err != nil {
		log.Error(err)
		return []retrievalmarket.RetrievalPeer{}
	}
	return peers
}

// TODO: Update to match spec for V0 epic
// https://github.com/filecoin-project/go-retrieval-market-project/issues/8
func (c *client) Query(ctx context.Context, p retrievalmarket.RetrievalPeer, pieceCID []byte, params retrievalmarket.QueryParams) (retrievalmarket.QueryResponse, error) {
	s, err := c.network.NewQueryStream(p.ID)
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}
	defer s.Close()

	err = s.WriteQuery(retrievalmarket.Query{
		PieceCID: pieceCID,
	})
	if err != nil {
		log.Warn(err)
		return retrievalmarket.QueryResponseUndefined, err
	}

	return s.ReadQueryResponse()
}

// TODO: Update to match spec for V0 Epic:
// https://github.com/filecoin-project/go-retrieval-market-project/issues/9
func (c *client) Retrieve(ctx context.Context, pieceCID []byte, params retrievalmarket.Params, totalFunds tokenamount.TokenAmount, miner peer.ID, clientWallet address.Address, minerWallet address.Address) retrievalmarket.DealID {
	/* The implementation of this function is just wrapper for the old code which retrieves UnixFS pieces
	-- it will be replaced when we do the V0 implementation of the module */
	c.nextDealLk.Lock()
	c.nextDealID++
	dealID := c.nextDealID
	c.nextDealLk.Unlock()

	dealState := retrievalmarket.ClientDealState{
		DealProposal: retrievalmarket.DealProposal{
			PieceCID: pieceCID,
			ID:       dealID,
			Params:   params,
		},
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

	environment := clientDealEnvironment{c.node, &UnixFs0Verifier{Root: dealState.DealProposal.PayloadCID}, c.bs, s}

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
			handler = clientstates.ProcessNextResponse
		default:
			c.failDeal(&dealState, xerrors.New("unexpected deal state"))
			return
		}
		dealModifier := handler(ctx, environment, dealState)
		dealModifier(&dealState)
		if retrievalmarket.IsTerminalStatus(dealState.Status) {
			break
		}
		c.notifySubscribers(retrievalmarket.ClientEventProgress, dealState)
	}
	if retrievalmarket.IsTerminalSuccess(dealState.Status) {
		c.notifySubscribers(retrievalmarket.ClientEventComplete, dealState)
	} else {
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

/*
type clientStream struct {
	node   retrievalmarket.RetrievalClientNode
	stream network.Stream
	peeker cbg.BytePeeker

	root   cid.Cid
	size   tokenamount.TokenAmount
	offset uint64

	paych       address.Address
	laneAddr        uint64
	total       tokenamount.TokenAmount
	transferred tokenamount.TokenAmount

	windowSize uint64 // how much we "trust" the peer
	verifier   BlockVerifier
	bs         blockstore.Blockstore
}
*/
/* This is the old retrieval code that is NOT spec compliant */

// C > S
//
// Offset MUST be aligned on chunking boundaries, size is rounded up to leaf size
//
// > DealProposal{Mode: Unixfs0, RootCid, Offset, Size, Payment(nil if free)}
// < Resp{Accept}
// < ..(Intermediate Block)
// < ..Blocks
// < ..(Intermediate Block)
// < ..Blocks
// > DealProposal(...)
// < ...
/*func (c *client) retrieveUnixfs(ctx context.Context, root cid.Cid, size uint64, total tokenamount.TokenAmount, miner peer.ID, client, minerAddr address.Address) error {
	s, err := c.h.NewStream(ctx, miner, retrievalmarket.ProtocolID)
	if err != nil {
		return xerrors.Errorf("failed to open stream to miner for retrieval query: %w", err)
	}
	defer s.Close()

	initialOffset := uint64(0) // TODO: Check how much data we have locally
	// TODO: Support in handler
	// TODO: Allow client to specify this

	paych, err := c.node.GetOrCreatePaymentChannel(ctx, client, minerAddr, total)
	if err != nil {
		return xerrors.Errorf("getting payment channel: %w", err)
	}
	laneAddr, err := c.node.AllocateLane(paych)
	if err != nil {
		return xerrors.Errorf("allocating payment laneAddr: %w", err)
	}

	cst := clientStream{
		node:   c.node,
		stream: s,
		peeker: cbg.GetPeeker(s),

		root:   root,
		size:   tokenamount.FromInt(size),
		offset: initialOffset,

		paych:       paych,
		laneAddr:        laneAddr,
		total:       total,
		transferred: tokenamount.FromInt(0),

		windowSize: params.UnixfsChunkSize,
		verifier:   &UnixFs0Verifier{Root: root},
		bs:         c.bs,
	}

	for cst.offset != size+initialOffset {
		toFetch := cst.windowSize
		if toFetch+cst.offset > size {
			toFetch = size - cst.offset
		}
		log.Infof("Retrieve %dB @%d", toFetch, cst.offset)

		err := cst.doOneExchange(ctx, toFetch)
		if err != nil {
			return xerrors.Errorf("retrieval exchange: %w", err)
		}

		cst.offset += toFetch
	}
	return nil
}

func (cst *clientStream) doOneExchange(ctx context.Context, toFetch uint64) error {
	payAmount := tokenamount.Div(tokenamount.Mul(cst.total, tokenamount.FromInt(toFetch)), cst.size)

	payment, err := cst.setupPayment(ctx, payAmount)
	if err != nil {
		return xerrors.Errorf("setting up retrieval payment: %w", err)
	}

	deal := &OldDealProposal{
		Payment: payment,
		Ref:     cst.root,
		Params: RetParams{
			Unixfs0: &Unixfs0Offer{
				Offset: cst.offset,
				Size:   toFetch,
			},
		},
	}

	if err := cborutil.WriteCborRPC(cst.stream, deal); err != nil {
		return xerrors.Errorf("sending incremental retrieval request: %w", err)
	}

	var resp OldDealResponse
	if err := cborutil.ReadCborRPC(cst.peeker, &resp); err != nil {
		return xerrors.Errorf("reading retrieval response: %w", err)
	}

	if resp.Status != Accepted {
		cst.windowSize = params.UnixfsChunkSize
		// TODO: apply some 'penalty' to miner 'reputation' (needs to be the same in both cases)

		if resp.Status == Error {
			return xerrors.Errorf("storage deal error: %s", resp.Message)
		}
		if resp.Status == Rejected {
			return xerrors.Errorf("storage deal rejected: %s", resp.Message)
		}
		return xerrors.New("storage deal response had no Accepted section")
	}

	log.Info("Retrieval accepted, fetching blocks")

	return cst.fetchBlocks(toFetch)

	// TODO: maybe increase miner window size after success
}

func (cst *clientStream) fetchBlocks(toFetch uint64) error {
	blocksToFetch := (toFetch + params.UnixfsChunkSize - 1) / params.UnixfsChunkSize

	for i := uint64(0); i < blocksToFetch; {
		log.Infof("block %d of %d", i+1, blocksToFetch)

		var block Block
		if err := cborutil.ReadCborRPC(cst.peeker, &block); err != nil {
			return xerrors.Errorf("reading fetchBlock response: %w", err)
		}

		dataBlocks, err := cst.consumeBlockMessage(block)
		if err != nil {
			return xerrors.Errorf("consuming retrieved blocks: %w", err)
		}

		i += dataBlocks
	}

	return nil
}

func (cst *clientStream) consumeBlockMessage(block Block) (uint64, error) {
	prefix, err := cid.PrefixFromBytes(block.Prefix)
	if err != nil {
		return 0, err
	}

	cid, err := prefix.Sum(block.Data)
	if err != nil {
		return 0, err
	}

	blk, err := blocks.NewBlockWithCid(block.Data, cid)
	if err != nil {
		return 0, err
	}

	internal, err := cst.verifier.Verify(context.TODO(), blk)
	if err != nil {
		log.Warnf("block verify failed: %s", err)
		return 0, err
	}

	// TODO: Smarter out, maybe add to filestore automagically
	//  (Also, persist intermediate nodes)
	err = cst.bs.Put(blk)
	if err != nil {
		log.Warnf("block write failed: %s", err)
		return 0, err
	}

	if internal {
		return 0, nil
	}

	return 1, nil
}

func (cst *clientStream) setupPayment(ctx context.Context, toSend tokenamount.TokenAmount) (OldPaymentInfo, error) {
	amount := tokenamount.Add(cst.transferred, toSend)

	sv, err := cst.node.CreatePaymentVoucher(ctx, cst.paych, amount, cst.laneAddr)
	if err != nil {
		return OldPaymentInfo{}, err
	}

	cst.transferred = amount

	return OldPaymentInfo{
		Channel:        cst.paych,
		ChannelMessage: nil,
		Vouchers:       []*types.SignedVoucher{sv},
	}, nil
}
*/

type clientDealEnvironment struct {
	node     retrievalmarket.RetrievalClientNode
	verifier BlockVerifier
	bs       blockstore.Blockstore
	stream   rmnet.RetrievalDealStream
}

func (cde clientDealEnvironment) Node() retrievalmarket.RetrievalClientNode {
	return cde.node
}

func (cde clientDealEnvironment) DealStream() rmnet.RetrievalDealStream {
	return cde.stream
}

func (cde clientDealEnvironment) ConsumeBlock(ctx context.Context, block retrievalmarket.Block) (uint64, bool, error) {
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
