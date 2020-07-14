package requestvalidation

import (
	"context"
	"errors"
	"sync"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

// RevalidatorEnvironment are the dependencies needed to
// build the logic of revalidation -- essentially, access to the node at statemachines
type RevalidatorEnvironment interface {
	Node() rm.RetrievalProviderNode
	SendEvent(dealID rm.ProviderDealIdentifier, evt rm.ProviderEvent, args ...interface{}) error
	Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error)
}

type channelData struct {
	dealID       rm.ProviderDealIdentifier
	totalSent    uint64
	totalPaidFor uint64
	interval     uint64
	pricePerByte abi.TokenAmount
	reload       bool
}

// ProviderRevalidator defines data transfer revalidation logic in the context of
// a provider for a retrieval deal
type ProviderRevalidator struct {
	env               RevalidatorEnvironment
	trackedChannelsLk sync.RWMutex
	trackedChannels   map[datatransfer.ChannelID]*channelData
}

// NewProviderRevalidator returns a new instance of a ProviderRevalidator
func NewProviderRevalidator(env RevalidatorEnvironment) *ProviderRevalidator {
	return &ProviderRevalidator{
		env:             env,
		trackedChannels: make(map[datatransfer.ChannelID]*channelData),
	}
}

// TrackChannel indicates a retrieval deal tracked by this provider. It associates
// a given channel ID with a retrieval deal, so that checks run for data sent
// on the channel
func (pr *ProviderRevalidator) TrackChannel(deal rm.ProviderDealState) {
	pr.trackedChannelsLk.Lock()
	defer pr.trackedChannelsLk.Unlock()
	pr.trackedChannels[deal.ChannelID] = &channelData{
		dealID: deal.Identifier(),
	}
	pr.writeDealState(deal)
}

func (pr *ProviderRevalidator) UntrackChannel(deal rm.ProviderDealState) {
	pr.trackedChannelsLk.Lock()
	defer pr.trackedChannelsLk.Unlock()
	delete(pr.trackedChannels, deal.ChannelID)
}

func (pr *ProviderRevalidator) loadDealState(channel *channelData) error {
	if !channel.reload {
		return nil
	}
	deal, err := pr.env.Get(channel.dealID)
	if err != nil {
		return err
	}
	pr.writeDealState(deal)
	channel.reload = false
	return nil
}

func (pr *ProviderRevalidator) writeDealState(deal rm.ProviderDealState) {
	channel := pr.trackedChannels[deal.ChannelID]
	channel.totalSent = deal.TotalSent
	channel.totalPaidFor = big.Div(deal.FundsReceived, deal.PricePerByte).Uint64()
	channel.interval = deal.CurrentInterval
	channel.pricePerByte = deal.PricePerByte
}

// Revalidate revalidates a request with a new voucher
func (pr *ProviderRevalidator) Revalidate(channelID datatransfer.ChannelID, voucher datatransfer.Voucher) (datatransfer.VoucherResult, error) {
	pr.trackedChannelsLk.RLock()
	defer pr.trackedChannelsLk.RUnlock()
	channel, ok := pr.trackedChannels[channelID]
	if !ok {
		return nil, nil
	}

	// read payment, or fail
	payment, ok := voucher.(*rm.DealPayment)
	if !ok {
		return nil, errors.New("wrong voucher type")
	}
	response, err := pr.processPayment(channel.dealID, payment)
	if err == nil {
		channel.reload = true
	}
	return response, err
}

func (pr *ProviderRevalidator) processPayment(dealID rm.ProviderDealIdentifier, payment *rm.DealPayment) (datatransfer.VoucherResult, error) {

	tok, _, err := pr.env.Node().GetChainHead(context.TODO())
	if err != nil {
		_ = pr.env.SendEvent(dealID, rm.ProviderEventSaveVoucherFailed, err)
		return errorDealResponse(dealID, err), err
	}

	deal, err := pr.env.Get(dealID)

	// attempt to redeem voucher
	// (totalSent * pricePerbyte) - fundsReceived
	paymentOwed := big.Sub(big.Mul(abi.NewTokenAmount(int64(deal.TotalSent)), deal.PricePerByte), deal.FundsReceived)
	received, err := pr.env.Node().SavePaymentVoucher(context.TODO(), payment.PaymentChannel, payment.PaymentVoucher, nil, paymentOwed, tok)
	if err != nil {
		_ = pr.env.SendEvent(dealID, rm.ProviderEventSaveVoucherFailed, err)
		return errorDealResponse(dealID, err), err
	}

	// received = 0 / err = nil indicates that the voucher was already saved, but this may be ok
	// if we are making a deal with ourself - in this case, we'll instead calculate received
	// but subtracting from fund sent
	if big.Cmp(received, big.Zero()) == 0 {
		received = big.Sub(payment.PaymentVoucher.Amount, deal.FundsReceived)
	}

	// check if all payments are received to continue the deal, or send updated required payment
	if received.LessThan(paymentOwed) {
		_ = pr.env.SendEvent(dealID, rm.ProviderEventPartialPaymentReceived, received)
		return &rm.DealResponse{
			ID:          deal.ID,
			Status:      deal.Status,
			PaymentOwed: big.Sub(paymentOwed, received),
		}, datatransfer.ErrPause
	}

	// resume deal
	_ = pr.env.SendEvent(dealID, rm.ProviderEventPaymentReceived, received)
	if deal.Status == rm.DealStatusFundsNeededLastPayment {
		return &rm.DealResponse{
			ID:     deal.ID,
			Status: rm.DealStatusCompleted,
		}, nil
	}
	return nil, nil
}

func errorDealResponse(dealID rm.ProviderDealIdentifier, err error) *rm.DealResponse {
	return &rm.DealResponse{
		ID:      dealID.DealID,
		Message: err.Error(),
		Status:  rm.DealStatusErrored,
	}
}

// OnPullDataSent is called on the responder side when more bytes are sent
// for a given pull request. It should return a VoucherResult + ErrPause to
// request revalidation or nil to continue uninterrupted,
// other errors will terminate the request
func (pr *ProviderRevalidator) OnPullDataSent(chid datatransfer.ChannelID, additionalBytesSent uint64) (datatransfer.VoucherResult, error) {
	pr.trackedChannelsLk.RLock()
	defer pr.trackedChannelsLk.RUnlock()
	channel, ok := pr.trackedChannels[chid]
	if !ok {
		return nil, nil
	}

	err := pr.loadDealState(channel)
	if err != nil {
		return nil, err
	}

	channel.totalSent += additionalBytesSent
	if channel.totalSent-channel.totalPaidFor >= channel.interval {
		paymentOwed := big.Mul(abi.NewTokenAmount(int64(channel.totalSent-channel.totalPaidFor)), channel.pricePerByte)
		err := pr.env.SendEvent(channel.dealID, rm.ProviderEventPaymentRequested, channel.totalSent)
		if err != nil {
			return nil, err
		}
		return &rm.DealResponse{
			ID:          channel.dealID.DealID,
			Status:      rm.DealStatusFundsNeeded,
			PaymentOwed: paymentOwed,
		}, datatransfer.ErrPause
	}
	return nil, pr.env.SendEvent(channel.dealID, rm.ProviderEventBlockSent, channel.totalSent)
}

// OnPushDataReceived is called on the responder side when more bytes are received
// for a given push request.  It should return a VoucherResult + ErrPause to
// request revalidation or nil to continue uninterrupted,
// other errors will terminate the request
func (pr *ProviderRevalidator) OnPushDataReceived(chid datatransfer.ChannelID, additionalBytesReceived uint64) (datatransfer.VoucherResult, error) {
	return nil, nil
}

// OnComplete is called to make a final request for revalidation -- often for the
// purpose of settlement.
// if VoucherResult is non nil, the request will enter a settlement phase awaiting
// a final update
func (pr *ProviderRevalidator) OnComplete(chid datatransfer.ChannelID) (datatransfer.VoucherResult, error) {
	pr.trackedChannelsLk.RLock()
	defer pr.trackedChannelsLk.RUnlock()
	channel, ok := pr.trackedChannels[chid]
	if !ok {
		return nil, nil
	}

	err := pr.loadDealState(channel)
	if err != nil {
		return nil, err
	}

	err = pr.env.SendEvent(channel.dealID, rm.ProviderEventBlocksCompleted)
	if err != nil {
		return nil, err
	}

	paymentOwed := big.Mul(abi.NewTokenAmount(int64(channel.totalSent-channel.totalPaidFor)), channel.pricePerByte)
	err = pr.env.SendEvent(channel.dealID, rm.ProviderEventPaymentRequested, channel.totalSent)
	if err != nil {
		return nil, err
	}
	return &rm.DealResponse{
		ID:          channel.dealID.DealID,
		Status:      rm.DealStatusFundsNeededLastPayment,
		PaymentOwed: paymentOwed,
	}, datatransfer.ErrPause
}
