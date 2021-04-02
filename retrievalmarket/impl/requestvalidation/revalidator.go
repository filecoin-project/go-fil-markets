package requestvalidation

import (
	"context"
	"errors"
	"sync"

	logging "github.com/ipfs/go-log/v2"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
)

var log = logging.Logger("retrieval-revalidator")

// RevalidatorEnvironment are the dependencies needed to
// build the logic of revalidation -- essentially, access to the node at statemachines
type RevalidatorEnvironment interface {
	Node() rm.RetrievalProviderNode
	SendEvent(dealID rm.ProviderDealIdentifier, evt rm.ProviderEvent, args ...interface{}) error
	Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error)
}

type channelData struct {
	dealID         rm.ProviderDealIdentifier
	totalSent      uint64
	totalPaidFor   uint64
	interval       uint64
	pricePerByte   abi.TokenAmount
	reload         bool
	legacyProtocol bool
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
	// Sanity check
	if deal.ChannelID == nil {
		log.Errorf("cannot track deal %s: channel ID is nil", deal.ID)
		return
	}

	pr.trackedChannelsLk.Lock()
	defer pr.trackedChannelsLk.Unlock()
	pr.trackedChannels[*deal.ChannelID] = &channelData{
		dealID: deal.Identifier(),
	}
	pr.writeDealState(deal)
}

// UntrackChannel indicates a retrieval deal is finish and no longer is tracked
// by this provider
func (pr *ProviderRevalidator) UntrackChannel(deal rm.ProviderDealState) {
	// Sanity check
	if deal.ChannelID == nil {
		log.Errorf("cannot untrack deal %s: channel ID is nil", deal.ID)
		return
	}

	pr.trackedChannelsLk.Lock()
	defer pr.trackedChannelsLk.Unlock()
	delete(pr.trackedChannels, *deal.ChannelID)
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
	channel := pr.trackedChannels[*deal.ChannelID]
	channel.totalSent = deal.TotalSent
	if !deal.PricePerByte.IsZero() {
		channel.totalPaidFor = big.Div(big.Max(big.Sub(deal.FundsReceived, deal.UnsealPrice), big.Zero()), deal.PricePerByte).Uint64()
	}
	channel.interval = deal.CurrentInterval
	channel.pricePerByte = deal.PricePerByte
	channel.legacyProtocol = deal.LegacyProtocol
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
	var legacyProtocol bool
	if !ok {
		legacyPayment, ok := voucher.(*migrations.DealPayment0)
		if !ok {
			return nil, errors.New("wrong voucher type")
		}
		newPayment := migrations.MigrateDealPayment0To1(*legacyPayment)
		payment = &newPayment
		legacyProtocol = true
	}

	response, err := pr.processPayment(channel.dealID, payment)
	if err == nil || err == datatransfer.ErrResume {
		channel.reload = true
	}
	return finalResponse(response, legacyProtocol), err
}

func (pr *ProviderRevalidator) processPayment(dealID rm.ProviderDealIdentifier, payment *rm.DealPayment) (*retrievalmarket.DealResponse, error) {

	tok, _, err := pr.env.Node().GetChainHead(context.TODO())
	if err != nil {
		_ = pr.env.SendEvent(dealID, rm.ProviderEventSaveVoucherFailed, err)
		return errorDealResponse(dealID, err), err
	}

	deal, err := pr.env.Get(dealID)
	if err != nil {
		return errorDealResponse(dealID, err), err
	}

	// attempt to redeem voucher
	// (totalSent * pricePerByte + unsealPrice) - fundsReceived
	paymentOwed := big.Sub(big.Add(big.Mul(abi.NewTokenAmount(int64(deal.TotalSent)), deal.PricePerByte), deal.UnsealPrice), deal.FundsReceived)
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
		}, datatransfer.ErrResume
	}

	// We shouldn't resume the data transfer if we haven't finished unsealing/reading the unsealed data into the
	// local block-store.
	if deal.Status == rm.DealStatusUnsealing || deal.Status == rm.DealStatusFundsNeededUnseal {
		return nil, nil
	}

	return nil, datatransfer.ErrResume
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
func (pr *ProviderRevalidator) OnPullDataSent(chid datatransfer.ChannelID, additionalBytesSent uint64) (bool, datatransfer.VoucherResult, error) {
	pr.trackedChannelsLk.RLock()
	defer pr.trackedChannelsLk.RUnlock()
	channel, ok := pr.trackedChannels[chid]
	if !ok {
		return false, nil, nil
	}

	err := pr.loadDealState(channel)
	if err != nil {
		return true, nil, err
	}

	channel.totalSent += additionalBytesSent
	if channel.pricePerByte.IsZero() || channel.totalSent-channel.totalPaidFor < channel.interval {
		return true, nil, pr.env.SendEvent(channel.dealID, rm.ProviderEventBlockSent, channel.totalSent)
	}

	paymentOwed := big.Mul(abi.NewTokenAmount(int64(channel.totalSent-channel.totalPaidFor)), channel.pricePerByte)
	err = pr.env.SendEvent(channel.dealID, rm.ProviderEventPaymentRequested, channel.totalSent)
	if err != nil {
		return true, nil, err
	}
	return true, finalResponse(&rm.DealResponse{
		ID:          channel.dealID.DealID,
		Status:      rm.DealStatusFundsNeeded,
		PaymentOwed: paymentOwed,
	}, channel.legacyProtocol), datatransfer.ErrPause
}

// OnPushDataReceived is called on the responder side when more bytes are received
// for a given push request.  It should return a VoucherResult + ErrPause to
// request revalidation or nil to continue uninterrupted,
// other errors will terminate the request
func (pr *ProviderRevalidator) OnPushDataReceived(chid datatransfer.ChannelID, additionalBytesReceived uint64) (bool, datatransfer.VoucherResult, error) {
	return false, nil, nil
}

// OnComplete is called to make a final request for revalidation -- often for the
// purpose of settlement.
// if VoucherResult is non nil, the request will enter a settlement phase awaiting
// a final update
func (pr *ProviderRevalidator) OnComplete(chid datatransfer.ChannelID) (bool, datatransfer.VoucherResult, error) {
	pr.trackedChannelsLk.RLock()
	defer pr.trackedChannelsLk.RUnlock()
	channel, ok := pr.trackedChannels[chid]
	if !ok {
		return false, nil, nil
	}

	err := pr.loadDealState(channel)
	if err != nil {
		return true, nil, err
	}

	err = pr.env.SendEvent(channel.dealID, rm.ProviderEventBlocksCompleted)
	if err != nil {
		return true, nil, err
	}

	paymentOwed := big.Mul(abi.NewTokenAmount(int64(channel.totalSent-channel.totalPaidFor)), channel.pricePerByte)
	if paymentOwed.Equals(big.Zero()) {
		return true, finalResponse(&rm.DealResponse{
			ID:     channel.dealID.DealID,
			Status: rm.DealStatusCompleted,
		}, channel.legacyProtocol), nil
	}
	err = pr.env.SendEvent(channel.dealID, rm.ProviderEventPaymentRequested, channel.totalSent)
	if err != nil {
		return true, nil, err
	}
	return true, finalResponse(&rm.DealResponse{
		ID:          channel.dealID.DealID,
		Status:      rm.DealStatusFundsNeededLastPayment,
		PaymentOwed: paymentOwed,
	}, channel.legacyProtocol), datatransfer.ErrPause
}

func finalResponse(response *rm.DealResponse, legacyProtocol bool) datatransfer.Voucher {
	if response == nil {
		return nil
	}
	if legacyProtocol {
		downgradedResponse := migrations.DealResponse0{
			Status:      response.Status,
			ID:          response.ID,
			Message:     response.Message,
			PaymentOwed: response.PaymentOwed,
		}
		return &downgradedResponse
	}
	return response
}

type legacyRevalidator struct {
	providerRevalidator *ProviderRevalidator
}

func (lrv *legacyRevalidator) Revalidate(channelID datatransfer.ChannelID, voucher datatransfer.Voucher) (datatransfer.VoucherResult, error) {
	return lrv.providerRevalidator.Revalidate(channelID, voucher)
}

func (lrv *legacyRevalidator) OnPullDataSent(chid datatransfer.ChannelID, additionalBytesSent uint64) (bool, datatransfer.VoucherResult, error) {
	return false, nil, nil
}

func (lrv *legacyRevalidator) OnPushDataReceived(chid datatransfer.ChannelID, additionalBytesReceived uint64) (bool, datatransfer.VoucherResult, error) {
	return false, nil, nil
}

func (lrv *legacyRevalidator) OnComplete(chid datatransfer.ChannelID) (bool, datatransfer.VoucherResult, error) {
	return false, nil, nil
}

// NewLegacyRevalidator adds a revalidator that will capture revalidation requests for the legacy protocol but
// won't double count data being sent
// TODO: the data transfer revalidator registration needs to be able to take multiple types to avoid double counting
// for data being sent.
func NewLegacyRevalidator(providerRevalidator *ProviderRevalidator) datatransfer.Revalidator {
	return &legacyRevalidator{providerRevalidator: providerRevalidator}
}
