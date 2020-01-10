package retrievalmarket

import (
	"context"
	"errors"
	"math/big"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	"github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/peer"
)

//go:generate cbor-gen-for Query QueryResponse DealProposal DealResponse Params QueryParams DealPayment Block ClientDealState

// type aliases
// TODO: Remove and use native types or extract for
// https://github.com/filecoin-project/go-retrieval-market-project/issues/5

// ProtocolID is the protocol for proposing / responding to retrieval deals
const ProtocolID = "/fil/retrieval/0.0.1"

// QueryProtocolID is the protocol for querying information about retrieval
// deal parameters
const QueryProtocolID = "/fil/retrieval/qry/0.0.1" // TODO: spec

// Unsubscribe is a function that unsubscribes a subscriber for either the
// client or the provider
type Unsubscribe func()

// ClientDealState is the current state of a deal from the point of view
// of a retrieval client
type ClientDealState struct {
	ProposalCid cid.Cid
	DealProposal
	TotalFunds       tokenamount.TokenAmount
	ClientWallet     address.Address
	MinerWallet      address.Address
	PayCh            address.Address
	Lane             uint64
	Status           DealStatus
	Sender           peer.ID
	TotalReceived    uint64
	Message          string
	BytesPaidFor     uint64
	CurrentInterval  uint64
	PaymentRequested tokenamount.TokenAmount
	FundsSpent       tokenamount.TokenAmount
}

// ClientEvent is an event that occurs in a deal lifecycle on the client
type ClientEvent uint64

const (
	// ClientEventOpen indicates a deal was initiated
	ClientEventOpen ClientEvent = iota

	// ClientEventFundsExpended indicates a deal has run out of funds in the payment channel
	// forcing the client to add more funds to continue the deal
	ClientEventFundsExpended // when totalFunds is expended

	// ClientEventProgress indicates more data was received for a retrieval
	ClientEventProgress

	// ClientEventError indicates an error occurred during a deal
	ClientEventError

	// ClientEventComplete indicates a deal has completed
	ClientEventComplete
)

// ClientSubscriber is a callback that is registered to listen for retrieval events
type ClientSubscriber func(event ClientEvent, state ClientDealState)

// RetrievalClient is a client interface for making retrieval deals
type RetrievalClient interface {
	// V0

	// Find Providers finds retrieval providers who may be storing a given piece
	FindProviders(pieceCID []byte) []RetrievalPeer

	// Query asks a provider for information about a piece it is storing
	Query(
		ctx context.Context,
		p RetrievalPeer,
		pieceCID []byte,
		params QueryParams,
	) (QueryResponse, error)

	// Retrieve retrieves all or part of a piece with the given retrieval parameters
	Retrieve(
		ctx context.Context,
		pieceCID []byte,
		params Params,
		totalFunds tokenamount.TokenAmount,
		miner peer.ID,
		clientWallet address.Address,
		minerWallet address.Address,
	) DealID

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ClientSubscriber) Unsubscribe

	// V1
	AddMoreFunds(id DealID, amount tokenamount.TokenAmount) error
	CancelDeal(id DealID) error
	RetrievalStatus(id DealID)
	ListDeals() map[DealID]ClientDealState
}

// RetrievalClientNode are the node dependencies for a RetrievalClient
type RetrievalClientNode interface {

	// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
	// between a client and a miner and insures the client has the given amount of funds available in the channel
	GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable tokenamount.TokenAmount) (address.Address, error)

	// Allocate late creates a lane within a payment channel so that calls to
	// CreatePaymentVoucher will automatically make vouchers only for the difference
	// in total
	AllocateLane(paymentChannel address.Address) (uint64, error)

	// CreatePaymentVoucher creates a new payment voucher in the given lane for a
	// given payment channel so that all the payment vouchers in the lane add up
	// to the given amount (so the payment voucher will be for the difference)
	CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount tokenamount.TokenAmount, lane uint64) (*types.SignedVoucher, error)
}

// ProviderDealState is the current state of a deal from the point of view
// of a retrieval provider
type ProviderDealState struct {
	DealProposal
	Status        DealStatus
	Receiver      peer.ID
	TotalSent     uint64
	FundsReceived tokenamount.TokenAmount
}

// ProviderEvent is an event that occurs in a deal lifecycle on the provider
type ProviderEvent uint64

const (

	// ProviderEventOpen indicates a new deal was received from a client
	ProviderEventOpen ProviderEvent = iota

	// ProviderEventProgress indicates more data was sent to a client
	ProviderEventProgress

	// ProviderEventError indicates an error occurred in processing a deal for a client
	ProviderEventError

	// ProviderEventComplete indicates a retrieval deal was completed for a client
	ProviderEventComplete
)

// ProviderDealID is a unique identifier for a deal on a provider -- it is
// a combination of DealID set by the client and the peer ID of the client
type ProviderDealID struct {
	From peer.ID
	ID   DealID
}

// ProviderSubscriber is a callback that is registered to listen for retrieval events on a provider
type ProviderSubscriber func(event ProviderEvent, state ProviderDealState)

// RetrievalProvider is an interface by which a provider configures their
// retrieval operations and monitors deals received and process
type RetrievalProvider interface {
	// Start begins listening for deals on the given host
	Start() error

	// V0

	// SetPricePerByte sets the price per byte a miner charges for retrievals
	SetPricePerByte(price tokenamount.TokenAmount)

	// SetPaymentInterval sets the maximum number of bytes a a provider will send before
	// requesting further payment, and the rate at which that value increases
	SetPaymentInterval(paymentInterval uint64, paymentIntervalIncrease uint64)

	// SubscribeToEvents listens for events that happen related to client retrievals
	SubscribeToEvents(subscriber ProviderSubscriber) Unsubscribe

	// V1
	SetPricePerUnseal(price tokenamount.TokenAmount)
	ListDeals() map[ProviderDealID]ProviderDealState
}

// RetrievalProviderNode are the node depedencies for a RetrevalProvider
type RetrievalProviderNode interface {
	GetPieceSize(pieceCid []byte) (uint64, error)
	SealedBlockstore(approveUnseal func() error) blockstore.Blockstore
	SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *types.SignedVoucher, proof []byte, expectedAmount tokenamount.TokenAmount) (tokenamount.TokenAmount, error)
}

// PeerResolver is an interface for looking up providers that may have a piece
type PeerResolver interface {
	GetPeers(data cid.Cid) ([]RetrievalPeer, error) // TODO: channel
}

// RetrievalPeer is a provider address/peer.ID pair (everything needed to make
// deals for with a miner)
type RetrievalPeer struct {
	Address address.Address
	ID      peer.ID // optional
}

// QueryResponseStatus indicates whether a queried piece is available
type QueryResponseStatus uint64

const (
	// QueryResponseAvailable indicates a provider has a piece and is prepared to
	// return it
	QueryResponseAvailable QueryResponseStatus = iota

	// QueryResponseUnavailable indicates a provider either does not have or cannot
	// serve the queried piece to the client
	QueryResponseUnavailable

	// QueryResponseError indicates something went wrong generating a query response
	QueryResponseError
)

// QueryItemStatus (V1) indicates whether the requested part of a piece (payload or selector)
// is available for retrieval
type QueryItemStatus uint64

const (
	// QueryItemAvailable indicates requested part of the piece is available to be
	// served
	QueryItemAvailable QueryItemStatus = iota

	// QueryItemUnavailable indicates the piece either does not contain the requested
	// item or it cannot be served
	QueryItemUnavailable

	// QueryItemUnknown indicates the provider cannot determine if the given item
	// is part of the requested piece (for example, if the piece is sealed and the
	// miner does not maintain a payload CID index)
	QueryItemUnknown
)

// QueryParams - V1 - indicate what specific information about a piece that a retrieval
// client is interested in, as well as specific parameters the client is seeking
// for the retrieval deal
type QueryParams struct {
	//PayloadCID                 cid.Cid   // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//Selector                   ipld.Node // optional, query if miner has this cid in this piece. some miners may not be able to respond.
	//MaxPricePerByte            tokenamount.TokenAmount    // optional, tell miner uninterested if more expensive than this
	//MinPaymentInterval         uint64    // optional, tell miner uninterested unless payment interval is greater than this
	//MinPaymentIntervalIncrease uint64    // optional, tell miner uninterested unless payment interval increase is greater than this
}

// Query is a query to a given provider to determine information about a piece
// they may have available for retrieval
type Query struct {
	PieceCID []byte // V0
	// QueryParams        // V1
}

// QueryUndefined is a query with no values
var QueryUndefined = Query{}

// NewQueryV0 creates a V0 query (which only specifies a piece)
func NewQueryV0(pieceCID []byte) Query {
	return Query{PieceCID: pieceCID}
}

// QueryResponse is a miners response to a given retrieval query
type QueryResponse struct {
	Status QueryResponseStatus
	//PayloadCIDFound QueryItemStatus // V1 - if a PayloadCid was requested, the result
	//SelectorFound   QueryItemStatus // V1 - if a Selector was requested, the result

	Size uint64 // Total size of piece in bytes
	//ExpectedPayloadSize uint64 // V1 - optional, if PayloadCID + selector are specified and miner knows, can offer an expected size

	PaymentAddress             address.Address // address to send funds to -- may be different than miner addr
	MinPricePerByte            tokenamount.TokenAmount
	MaxPaymentInterval         uint64
	MaxPaymentIntervalIncrease uint64
	Message                    string
}

// QueryResponseUndefined is an empty QueryResponse
var QueryResponseUndefined = QueryResponse{}

// PieceRetrievalPrice is the total price to retrieve the piece (size * MinPricePerByte)
func (qr QueryResponse) PieceRetrievalPrice() tokenamount.TokenAmount {
	return tokenamount.Mul(qr.MinPricePerByte, tokenamount.FromInt(qr.Size))
}

// PayloadRetrievalPrice is the expected price to retrieve just the given payload
// & selector (V1)
//func (qr QueryResponse) PayloadRetrievalPrice() tokenamount.TokenAmount {
//	return types.BigMul(qr.MinPricePerByte, types.NewInt(qr.ExpectedPayloadSize))
//}

// DealStatus is the status of a retrieval deal returned by a provider
// in a DealResponse
type DealStatus uint64

const (
	// DealStatusNew is a deal that nothing has happened with yet
	DealStatusNew DealStatus = iota

	// DealStatusPaymentChannelCreated is a deal status that has a payment channel
	// & lane setup
	DealStatusPaymentChannelCreated

	// DealStatusAccepted means a deal has been accepted by a provider
	// and its is ready to proceed with retrieval
	DealStatusAccepted

	// DealStatusFailed indicates something went wrong during a retrieval
	DealStatusFailed

	// DealStatusRejected indicates the provider rejected a client's deal proposal
	// for some reason
	DealStatusRejected

	// DealStatusUnsealing indicates the provider is currently unsealing the sector
	// needed to serve the retrieval deal
	DealStatusUnsealing

	// DealStatusFundsNeeded indicates the provider is awaiting a payment voucher to
	// continue processing the deal
	DealStatusFundsNeeded

	// DealStatusOngoing indicates the provider is continuing to process a deal
	DealStatusOngoing

	// DealStatusFundsNeededLastPayment indicates the provider is awaiting funds for
	// a final payment in order to complete a deal
	DealStatusFundsNeededLastPayment

	// DealStatusCompleted indicates a deal is complete
	DealStatusCompleted

	// DealStatusDealNotFound indicates an update was received for a deal that could
	// not be identified
	DealStatusDealNotFound
)

// IsTerminalError returns true if this status indicates processing of this deal
// is complete with an error
func IsTerminalError(status DealStatus) bool {
	return status == DealStatusDealNotFound ||
		status == DealStatusFailed ||
		status == DealStatusRejected
}

// IsTerminalSuccess returns true if this status indicates processing of this deal
// is complete with a success
func IsTerminalSuccess(status DealStatus) bool {
	return status == DealStatusCompleted
}

// IsTerminalStatus returns true if this status indicates processing of a deal is
// complete (either success or error)
func IsTerminalStatus(status DealStatus) bool {
	return IsTerminalError(status) || IsTerminalSuccess(status)
}

// Params are the parameters requested for a retrieval deal proposal
type Params struct {
	PayloadCID cid.Cid
	//Selector                ipld.Node // V1
	PricePerByte            tokenamount.TokenAmount
	PaymentInterval         uint64
	PaymentIntervalIncrease uint64
}

// NewParamsV0 generates parameters for a retrieval deal, which is always a whole piece deal
func NewParamsV0(pricePerByte *big.Int, paymentInterval uint64, paymentIntervalIncrease uint64) Params {
	return Params{
		PricePerByte:            tokenamount.TokenAmount{Int: pricePerByte},
		PaymentInterval:         paymentInterval,
		PaymentIntervalIncrease: paymentIntervalIncrease,
	}
}

// DealID is an identifier for a retrieval deal (unique to a client)
type DealID uint64

// DealProposal is a proposal for a new retrieval deal
type DealProposal struct {
	PieceCID []byte
	ID       DealID
	Params
}

// DealProposalUndefined is an undefined deal proposal
var DealProposalUndefined = DealProposal{}

// Block is an IPLD block in bitswap format
type Block struct {
	Prefix []byte
	Data   []byte
}

// DealResponse is a response to a retrieval deal proposal
type DealResponse struct {
	Status DealStatus
	ID     DealID

	// payment required to proceed
	PaymentOwed tokenamount.TokenAmount

	Message string
	Blocks  []Block // V0 only
}

// DealResponseUndefined is an undefined deal response
var DealResponseUndefined = DealResponse{}

// DealPayment is a payment for an in progress retrieval deal
type DealPayment struct {
	ID             DealID
	PaymentChannel address.Address
	PaymentVoucher *types.SignedVoucher
}

// DealPaymentUndefined is an undefined deal payment
var DealPaymentUndefined = DealPayment{}

var (
	// ErrNotFound means a piece was not found during retrieval
	ErrNotFound = errors.New("not found")
)
