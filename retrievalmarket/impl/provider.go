package retrievalimpl

import (
	"context"
	"reflect"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p-core/network"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
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

type handlerDeal struct { // nolint: unused
	p      *provider
	stream network.Stream

	ufsr UnixfsReader
	open cid.Cid
	at   uint64
	size uint64
}

// TODO: Update for https://github.com/filecoin-project/go-retrieval-market-project/issues/7
func (p *provider) HandleDealStream(stream rmnet.RetrievalDealStream) {
	defer stream.Close()
	/*
		hnd := &handlerDeal{
			p: p,

			stream: stream,
		}

		var err error
		more := true

		for more {
			more, err = hnd.handleNext() // TODO: 'more' bool
			if err != nil {
				writeErr(stream, err)
				return
			}
		}
	*/
}

/*
func (hnd *handlerDeal) handleNext() (bool, error) {
	var deal OldDealProposal
	if err := cborutil.ReadCborRPC(hnd.stream, &deal); err != nil {
		if err == io.EOF { // client sent all deals
			err = nil
		}
		return false, err
	}

	if deal.Params.Unixfs0 == nil {
		return false, xerrors.New("unknown deal type")
	}

	unixfs0 := deal.Params.Unixfs0

	if len(deal.Payment.Vouchers) != 1 {
		return false, xerrors.Errorf("expected one signed voucher, got %d", len(deal.Payment.Vouchers))
	}

	expPayment := tokenamount.Mul(hnd.p.pricePerByte, tokenamount.FromInt(deal.Params.Unixfs0.Size))
	if _, err := hnd.p.node.SavePaymentVoucher(context.TODO(), deal.Payment.Channel, deal.Payment.Vouchers[0], nil, expPayment); err != nil {
		return false, xerrors.Errorf("processing retrieval payment: %w", err)
	}

	// If the file isn't open (new deal stream), isn't the right file, or isn't
	// at the right offset, (re)open it
	if hnd.open != deal.Ref || hnd.at != unixfs0.Offset {
		log.Infof("opening file for sending (open '%s') (@%d, want %d)", deal.Ref, hnd.at, unixfs0.Offset)
		if err := hnd.openFile(deal); err != nil {
			return false, err
		}
	}

	if unixfs0.Offset+unixfs0.Size > hnd.size {
		return false, xerrors.Errorf("tried to read too much %d+%d > %d", unixfs0.Offset, unixfs0.Size, hnd.size)
	}

	err := hnd.accept(deal)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (hnd *handlerDeal) openFile(deal OldDealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	if unixfs0.Offset != 0 {
		// TODO: Implement SeekBlock (like ReadBlock) in go-unixfs
		return xerrors.New("sending merkle proofs for nonzero offset not supported yet")
	}
	hnd.at = unixfs0.Offset

	bstore := hnd.p.node.SealedBlockstore(func() error {
		return nil // TODO: approve unsealing based on amount paid
	})

	ds := merkledag.NewDAGService(blockservice.New(bstore, nil))
	rootNd, err := ds.Get(context.TODO(), deal.Ref)
	if err != nil {
		return err
	}

	fsr, err := unixfile.NewUnixfsFile(context.TODO(), ds, rootNd)
	if err != nil {
		return err
	}

	var ok bool
	hnd.ufsr, ok = fsr.(UnixfsReader)
	if !ok {
		return xerrors.Errorf("file %s didn't implement UnixfsReader", deal.Ref)
	}

	isize, err := hnd.ufsr.Size()
	if err != nil {
		return err
	}
	hnd.size = uint64(isize)

	hnd.open = deal.Ref

	return nil
}

func (hnd *handlerDeal) accept(deal OldDealProposal) error {
	unixfs0 := deal.Params.Unixfs0

	resp := &OldDealResponse{
		Status: Accepted,
	}
	if err := cborutil.WriteCborRPC(hnd.stream, resp); err != nil {
		log.Errorf("Retrieval query: Write Accepted resp: %s", err)
		return err
	}

	blocksToSend := (unixfs0.Size + params.UnixfsChunkSize - 1) / params.UnixfsChunkSize
	for i := uint64(0); i < blocksToSend; {
		data, offset, nd, err := hnd.ufsr.ReadBlock(context.TODO())
		if err != nil {
			return err
		}

		log.Infof("sending block for a deal: %s", nd.Cid())

		if offset != unixfs0.Offset {
			return xerrors.Errorf("ReadBlock on wrong offset: want %d, got %d", unixfs0.Offset, offset)
		}

		if uint64(len(data)) != deal.Unixfs0.Size { // TODO: Fix for internal nodes (and any other node too)
			writeErr(stream, xerrors.Errorf("ReadBlock data with wrong size: want %d, got %d", deal.Unixfs0.Size, len(data)))
			return
		}

		block := &Block{
			Prefix: nd.Cid().Prefix().Bytes(),
			Data:   nd.RawData(),
		}

		if err := cborutil.WriteCborRPC(hnd.stream, block); err != nil {
			return err
		}

		if len(data) > 0 { // don't count internal nodes
			hnd.at += uint64(len(data))
			i++
		}
	}

	return nil
}
*/
