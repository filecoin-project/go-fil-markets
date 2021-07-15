package storageimpl

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/ffiwrapper"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedfsm "github.com/filecoin-project/go-ds-versioning/pkg/fsm"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/carstore"
	mktdagstore "github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/connmanager"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

var _ storagemarket.StorageProvider = &Provider{}
var _ network.StorageReceiver = &Provider{}

// StoredAsk is an interface which provides access to a StorageAsk
type StoredAsk interface {
	GetAsk() *storagemarket.SignedStorageAsk
	SetAsk(price abi.TokenAmount, verifiedPrice abi.TokenAmount, duration abi.ChainEpoch, options ...storagemarket.StorageAskOption) error
}

// Provider is the production implementation of the StorageProvider interface
type Provider struct {
	net network.StorageMarketNetwork

	spn                       storagemarket.StorageProviderNode
	fs                        filestore.FileStore
	pieceStore                piecestore.PieceStore
	conns                     *connmanager.ConnManager
	storedAsk                 StoredAsk
	actor                     address.Address
	dataTransfer              datatransfer.Manager
	universalRetrievalEnabled bool
	customDealDeciderFunc     DealDeciderFunc
	pubSub                    *pubsub.PubSub
	readyMgr                  *shared.ReadyManager

	deals        fsm.Group
	migrateDeals func(context.Context) error

	unsubDataTransfer datatransfer.Unsubscribe

	shardReg             *ShardRegistration
	dagStore             mktdagstore.DagStoreWrapper
	readWriteBlockStores *carstore.CarReadWriteStoreTracker
}

// StorageProviderOption allows custom configuration of a storage provider
type StorageProviderOption func(p *Provider)

// EnableUniversalRetrieval causes a storage provider to track all CIDs in a piece,
// so that any CID, not just the root payload CID, can be retrieved
func EnableUniversalRetrieval() StorageProviderOption {
	return func(p *Provider) {
		p.universalRetrievalEnabled = true
	}
}

// DealDeciderFunc is a function which evaluates an incoming deal to decide if
// it its accepted
// It returns:
// - boolean = true if deal accepted, false if rejected
// - string = reason deal was not excepted, if rejected
// - error = if an error occurred trying to decide
type DealDeciderFunc func(context.Context, storagemarket.MinerDeal) (bool, string, error)

// CustomDealDecisionLogic allows a provider to call custom decision logic when validating incoming
// deal proposals
func CustomDealDecisionLogic(decider DealDeciderFunc) StorageProviderOption {
	return func(p *Provider) {
		p.customDealDeciderFunc = decider
	}
}

// NewProvider returns a new storage provider
func NewProvider(net network.StorageMarketNetwork,
	ds datastore.Batching,
	fs filestore.FileStore,
	dagStore mktdagstore.DagStoreWrapper,
	pieceStore piecestore.PieceStore,
	dataTransfer datatransfer.Manager,
	spn storagemarket.StorageProviderNode,
	minerAddress address.Address,
	storedAsk StoredAsk,
	shardReg *ShardRegistration,
	options ...StorageProviderOption,
) (storagemarket.StorageProvider, error) {
	h := &Provider{
		net:                  net,
		spn:                  spn,
		fs:                   fs,
		pieceStore:           pieceStore,
		conns:                connmanager.NewConnManager(),
		storedAsk:            storedAsk,
		actor:                minerAddress,
		dataTransfer:         dataTransfer,
		pubSub:               pubsub.New(providerDispatcher),
		readyMgr:             shared.NewReadyManager(),
		shardReg:             shardReg,
		dagStore:             dagStore,
		readWriteBlockStores: carstore.NewCarReadWriteStoreTracker(),
	}
	storageMigrations, err := migrations.ProviderMigrations.Build()
	if err != nil {
		return nil, err
	}
	h.deals, h.migrateDeals, err = newProviderStateMachine(
		ds,
		&providerDealEnvironment{h},
		h.dispatch,
		storageMigrations,
		versioning.VersionKey("1"),
	)
	if err != nil {
		return nil, err
	}
	h.Configure(options...)

	// register a data transfer event handler -- this will send events to the state machines based on DT events
	h.unsubDataTransfer = dataTransfer.SubscribeToEvents(dtutils.ProviderDataTransferSubscriber(h.deals))

	err = dataTransfer.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, requestvalidation.NewUnifiedRequestValidator(&providerPushDeals{h}, nil))
	if err != nil {
		return nil, err
	}

	err = dataTransfer.RegisterTransportConfigurer(&requestvalidation.StorageDataTransferVoucher{}, dtutils.TransportConfigurer(&providerStoreGetter{h}))
	if err != nil {
		return nil, err
	}

	return h, nil
}

// Start initializes deal processing on a StorageProvider and restarts in progress deals.
// It also registers the provider with a StorageMarketNetwork so it can receive incoming
// messages on the storage market's libp2p protocols
func (p *Provider) Start(ctx context.Context) error {
	err := p.net.SetDelegate(p)
	if err != nil {
		return err
	}
	go func() {
		err := p.start(ctx)
		if err != nil {
			log.Error(err.Error())
		}
	}()
	return nil
}

// OnReady registers a listener for when the provider has finished starting up
func (p *Provider) OnReady(ready shared.ReadyFunc) {
	p.readyMgr.OnReady(ready)
}

func (p *Provider) AwaitReady() error {
	return p.readyMgr.AwaitReady()
}

/*
HandleDealStream is called by the network implementation whenever a new message is received on the deal protocol

It initiates the provider side of the deal flow.

When a provider receives a DealProposal of the deal protocol, it takes the following steps:

1. Calculates the CID for the received ClientDealProposal.

2. Constructs a MinerDeal to track the state of this deal.

3. Tells its statemachine to begin tracking this deal state by CID of the received ClientDealProposal

4. Tracks the received deal stream by the CID of the ClientDealProposal

4. Triggers a `ProviderEventOpen` event on its statemachine.

From then on, the statemachine controls the deal flow in the client. Other components may listen for events in this flow by calling
`SubscribeToEvents` on the Provider. The Provider handles loading the next block to send to the client.

Documentation of the client state machine can be found at https://godoc.org/github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates
*/
func (p *Provider) HandleDealStream(s network.StorageDealStream) {
	log.Info("Handling storage deal proposal!")

	err := p.receiveDeal(s)
	if err != nil {
		log.Errorf("%+v", err)
		s.Close()
		return
	}
}

// TODO Write a one time script that registers shards for all Pieces that a miner has.
func (p *Provider) receiveDeal(s network.StorageDealStream) error {
	proposal, err := s.ReadDealProposal()
	if err != nil {
		return xerrors.Errorf("failed to read proposal message: %w", err)
	}

	proposalNd, err := cborutil.AsIpld(proposal.DealProposal)
	if err != nil {
		return err
	}

	// Check if we are already tracking this deal
	var md storagemarket.MinerDeal
	if err := p.deals.Get(proposalNd.Cid()).Get(&md); err == nil {
		// We are already tracking this deal, for some reason it was re-proposed, perhaps because of a client restart
		// this is ok, just send a response back.
		return p.resendProposalResponse(s, &md)
	}

	var carV2FilePath string
	// create an empty CARv2 file at a temp location that Graphysnc will write the incoming blocks to via a CARv2 ReadWrite blockstore wrapper.
	if proposal.Piece.TransferType != storagemarket.TTManual {
		tmp, err := p.fs.CreateTemp()
		if err != nil {
			return xerrors.Errorf("failed to create an empty temp CARv2 file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(string(tmp.OsPath()))
			return xerrors.Errorf("failed to close temp file: %w", err)
		}
		carV2FilePath = string(tmp.OsPath())
	}

	deal := &storagemarket.MinerDeal{
		Client:             s.RemotePeer(),
		Miner:              p.net.ID(),
		ClientDealProposal: *proposal.DealProposal,
		ProposalCid:        proposalNd.Cid(),
		State:              storagemarket.StorageDealUnknown,
		Ref:                proposal.Piece,
		FastRetrieval:      proposal.FastRetrieval,
		CreationTime:       curTime(),
		CARv2FilePath:      carV2FilePath,
	}

	err = p.deals.Begin(proposalNd.Cid(), deal)
	if err != nil {
		return err
	}
	err = p.conns.AddStream(proposalNd.Cid(), s)
	if err != nil {
		return err
	}
	return p.deals.Send(proposalNd.Cid(), storagemarket.ProviderEventOpen)
}

// Stop terminates processing of deals on a StorageProvider
func (p *Provider) Stop() error {
	p.readyMgr.Stop()
	p.unsubDataTransfer()
	err := p.deals.Stop(context.TODO())
	if err != nil {
		return err
	}
	return p.net.StopHandlingRequests()
}

// ImportDataForDeal manually imports data for an offline storage deal
// It will verify that the data in the passed io.Reader matches the expected piece
// cid for the given deal or it will error
func (p *Provider) ImportDataForDeal(ctx context.Context, propCid cid.Cid, data io.Reader) error {
	// TODO: be able to check if we have enough disk space
	var d storagemarket.MinerDeal
	if err := p.deals.Get(propCid).Get(&d); err != nil {
		return xerrors.Errorf("failed getting deal %s: %w", propCid, err)
	}

	tempfi, err := p.fs.CreateTemp()
	if err != nil {
		return xerrors.Errorf("failed to create temp file for data import: %w", err)
	}
	cleanup := func() {
		_ = tempfi.Close()
		_ = p.fs.Delete(tempfi.Path())
	}

	n, err := io.Copy(tempfi, data)
	if err != nil {
		cleanup()
		return xerrors.Errorf("importing deal data failed: %w", err)
	}

	_ = n // TODO: verify n?

	pieceSize := uint64(tempfi.Size())

	_, err = tempfi.Seek(0, io.SeekStart)
	if err != nil {
		cleanup()
		return xerrors.Errorf("failed to seek through temp imported file: %w", err)
	}

	proofType, err := p.spn.GetProofType(ctx, p.actor, nil)
	if err != nil {
		cleanup()
		return xerrors.Errorf("failed to determine proof type: %w", err)
	}

	pieceCid, err := generatePieceCommitment(proofType, tempfi, pieceSize)
	if err != nil {
		cleanup()
		return xerrors.Errorf("failed to generate commP: %w", err)
	}

	// Verify CommP matches
	if !pieceCid.Equals(d.Proposal.PieceCID) {
		cleanup()
		return xerrors.Errorf("given data does not match expected commP (got: %x, expected %x)", pieceCid, d.Proposal.PieceCID)
	}

	return p.deals.Send(propCid, storagemarket.ProviderEventVerifiedData, tempfi.Path(), filestore.Path(""))
}

func generatePieceCommitment(rt abi.RegisteredSealProof, rd io.Reader, pieceSize uint64) (cid.Cid, error) {
	paddedReader, paddedSize := padreader.New(rd, pieceSize)
	commitment, err := ffiwrapper.GeneratePieceCIDFromFile(rt, paddedReader, paddedSize)
	if err != nil {
		return cid.Undef, err
	}
	return commitment, nil
}

// GetAsk returns the storage miner's ask, or nil if one does not exist.
func (p *Provider) GetAsk() *storagemarket.SignedStorageAsk {
	return p.storedAsk.GetAsk()
}

// AddStorageCollateral adds storage collateral
func (p *Provider) AddStorageCollateral(ctx context.Context, amount abi.TokenAmount) error {
	done := make(chan error, 1)

	mcid, err := p.spn.AddFunds(ctx, p.actor, amount)
	if err != nil {
		return err
	}

	err = p.spn.WaitForMessage(ctx, mcid, func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error {
		if err != nil {
			done <- xerrors.Errorf("AddFunds errored: %w", err)
		} else if code != exitcode.Ok {
			done <- xerrors.Errorf("AddFunds error, exit code: %s", code.String())
		} else {
			done <- nil
		}
		return nil
	})

	if err != nil {
		return err
	}

	return <-done
}

// GetStorageCollateral returns the current collateral balance
func (p *Provider) GetStorageCollateral(ctx context.Context) (storagemarket.Balance, error) {
	tok, _, err := p.spn.GetChainHead(ctx)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return p.spn.GetBalance(ctx, p.actor, tok)
}

// ListLocalDeals lists deals processed by this storage provider
func (p *Provider) ListLocalDeals() ([]storagemarket.MinerDeal, error) {
	var out []storagemarket.MinerDeal
	if err := p.deals.List(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetAsk configures the storage miner's ask with the provided price,
// duration, and options. Any previously-existing ask is replaced.
func (p *Provider) SetAsk(price abi.TokenAmount, verifiedPrice abi.TokenAmount, duration abi.ChainEpoch, options ...storagemarket.StorageAskOption) error {
	return p.storedAsk.SetAsk(price, verifiedPrice, duration, options...)
}

/*
HandleAskStream is called by the network implementation whenever a new message is received on the ask protocol

A Provider handling a `AskRequest` does the following:

1. Reads the current signed storage ask from storage

2. Wraps the signed ask in an AskResponse and writes it on the StorageAskStream

The connection is kept open only as long as the request-response exchange.
*/
func (p *Provider) HandleAskStream(s network.StorageAskStream) {
	defer s.Close()
	ar, err := s.ReadAskRequest()
	if err != nil {
		log.Errorf("failed to read AskRequest from incoming stream: %s", err)
		return
	}

	var ask *storagemarket.SignedStorageAsk
	if p.actor != ar.Miner {
		log.Warnf("storage provider for address %s receive ask for miner with address %s", p.actor, ar.Miner)
	} else {
		ask = p.storedAsk.GetAsk()
	}

	resp := network.AskResponse{
		Ask: ask,
	}

	if err := s.WriteAskResponse(resp, p.sign); err != nil {
		log.Errorf("failed to write ask response: %s", err)
		return
	}
}

/*
HandleDealStatusStream is called by the network implementation whenever a new message is received on the deal status protocol

A Provider handling a `DealStatuRequest` does the following:

1. Lots the deal state from the Provider FSM

2. Verifies the signature on the DealStatusRequest matches the Client for this deal

3. Constructs a ProviderDealState from the deal state

4. Signs the ProviderDealState with its private key

5. Writes a DealStatusResponse with the ProviderDealState and signature onto the DealStatusStream

The connection is kept open only as long as the request-response exchange.
*/
func (p *Provider) HandleDealStatusStream(s network.DealStatusStream) {
	ctx := context.TODO()
	defer s.Close()
	request, err := s.ReadDealStatusRequest()
	if err != nil {
		log.Errorf("failed to read DealStatusRequest from incoming stream: %s", err)
		return
	}

	dealState, err := p.processDealStatusRequest(ctx, &request)
	if err != nil {
		log.Errorf("failed to process deal status request: %s", err)
		dealState = &storagemarket.ProviderDealState{
			State:   storagemarket.StorageDealError,
			Message: err.Error(),
		}
	}

	signature, err := p.sign(ctx, dealState)
	if err != nil {
		log.Errorf("failed to sign deal status response: %s", err)
		return
	}

	response := network.DealStatusResponse{
		DealState: *dealState,
		Signature: *signature,
	}

	if err := s.WriteDealStatusResponse(response, p.sign); err != nil {
		log.Warnf("failed to write deal status response: %s", err)
		return
	}
}

func (p *Provider) processDealStatusRequest(ctx context.Context, request *network.DealStatusRequest) (*storagemarket.ProviderDealState, error) {
	// fetch deal state
	var md = storagemarket.MinerDeal{}
	if err := p.deals.Get(request.Proposal).Get(&md); err != nil {
		log.Errorf("proposal doesn't exist in state store: %s", err)
		return nil, xerrors.Errorf("no such proposal")
	}

	// verify query signature
	buf, err := cborutil.Dump(&request.Proposal)
	if err != nil {
		log.Errorf("failed to serialize status request: %s", err)
		return nil, xerrors.Errorf("internal error")
	}

	tok, _, err := p.spn.GetChainHead(ctx)
	if err != nil {
		log.Errorf("failed to get chain head: %s", err)
		return nil, xerrors.Errorf("internal error")
	}

	err = providerutils.VerifySignature(ctx, request.Signature, md.ClientDealProposal.Proposal.Client, buf, tok, p.spn.VerifySignature)
	if err != nil {
		log.Errorf("invalid deal status request signature: %s", err)
		return nil, xerrors.Errorf("internal error")
	}

	return &storagemarket.ProviderDealState{
		State:         md.State,
		Message:       md.Message,
		Proposal:      &md.Proposal,
		ProposalCid:   &md.ProposalCid,
		AddFundsCid:   md.AddFundsCid,
		PublishCid:    md.PublishCid,
		DealID:        md.DealID,
		FastRetrieval: md.FastRetrieval,
	}, nil
}

// Configure applies the given list of StorageProviderOptions after a StorageProvider
// is initialized
func (p *Provider) Configure(options ...StorageProviderOption) {
	for _, option := range options {
		option(p)
	}
}

// UniversalRetrievalEnabled returns whether or not universal retrieval
// (retrieval by any CID, not just the root payload CID) is enabled
// for this provider
func (p *Provider) UniversalRetrievalEnabled() bool {
	return p.universalRetrievalEnabled
}

// SubscribeToEvents allows another component to listen for events on the StorageProvider
// in order to track deals as they progress through the deal flow
func (p *Provider) SubscribeToEvents(subscriber storagemarket.ProviderSubscriber) shared.Unsubscribe {
	return shared.Unsubscribe(p.pubSub.Subscribe(subscriber))
}

// dispatch puts the fsm event into a form that pubSub can consume,
// then publishes the event
func (p *Provider) dispatch(eventName fsm.EventName, deal fsm.StateType) {
	evt, ok := eventName.(storagemarket.ProviderEvent)
	if !ok {
		log.Errorf("dropped bad event %s", eventName)
	}
	realDeal, ok := deal.(storagemarket.MinerDeal)
	if !ok {
		log.Errorf("not a MinerDeal %v", deal)
	}
	pubSubEvt := internalProviderEvent{evt, realDeal}

	if err := p.pubSub.Publish(pubSubEvt); err != nil {
		log.Errorf("failed to publish event %d", evt)
	}
}

func (p *Provider) start(ctx context.Context) error {
	err := p.migrateDeals(ctx)
	publishErr := p.readyMgr.FireReady(err)
	if publishErr != nil {
		log.Warnf("Publish storage provider ready event: %s", err.Error())
	}
	if err != nil {
		return fmt.Errorf("Migrating storage provider state machines: %w", err)
	}

	var deals []storagemarket.MinerDeal
	err = p.deals.List(&deals)
	if err != nil {
		return xerrors.Errorf("failed to fetch deals during startup: %w", err)
	}
	if err := p.shardReg.registerShards(ctx, deals); err != nil {
		return fmt.Errorf("Failed to restart deals: %w", err)
	}
	if err := p.restartDeals(deals); err != nil {
		return fmt.Errorf("Failed to restart deals: %w", err)
	}
	return nil
}

func (p *Provider) restartDeals(deals []storagemarket.MinerDeal) error {
	for _, deal := range deals {
		if p.deals.IsTerminated(deal) {
			continue
		}

		err := p.deals.Send(deal.ProposalCid, storagemarket.ProviderEventRestart)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) sign(ctx context.Context, data interface{}) (*crypto.Signature, error) {
	tok, _, err := p.spn.GetChainHead(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get chain head: %w", err)
	}

	return providerutils.SignMinerData(ctx, data, p.actor, tok, p.spn.GetMinerWorkerAddress, p.spn.SignBytes)
}

func (p *Provider) resendProposalResponse(s network.StorageDealStream, md *storagemarket.MinerDeal) error {
	resp := &network.Response{State: md.State, Message: md.Message, Proposal: md.ProposalCid}
	sig, err := p.sign(context.TODO(), resp)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	err = s.WriteDealResponse(network.SignedResponse{Response: *resp, Signature: sig}, p.sign)

	if closeErr := s.Close(); closeErr != nil {
		log.Warnf("closing connection: %v", err)
	}

	return err
}

func newProviderStateMachine(ds datastore.Batching, env fsm.Environment, notifier fsm.Notifier, storageMigrations versioning.VersionedMigrationList, target versioning.VersionKey) (fsm.Group, func(context.Context) error, error) {
	return versionedfsm.NewVersionedFSM(ds, fsm.Parameters{
		Environment:     env,
		StateType:       storagemarket.MinerDeal{},
		StateKeyField:   "State",
		Events:          providerstates.ProviderEvents,
		StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
		FinalityStates:  providerstates.ProviderFinalityStates,
		Notifier:        notifier,
	}, storageMigrations, target)
}

type internalProviderEvent struct {
	evt  storagemarket.ProviderEvent
	deal storagemarket.MinerDeal
}

func providerDispatcher(evt pubsub.Event, fn pubsub.SubscriberFn) error {
	ie, ok := evt.(internalProviderEvent)
	if !ok {
		return xerrors.New("wrong type of event")
	}
	cb, ok := fn.(storagemarket.ProviderSubscriber)
	if !ok {
		return xerrors.New("wrong type of callback")
	}
	cb(ie.evt, ie.deal)
	return nil
}

// ProviderFSMParameterSpec is a valid set of parameters for a provider FSM - used in doc generation
var ProviderFSMParameterSpec = fsm.Parameters{
	Environment:     &providerDealEnvironment{},
	StateType:       storagemarket.MinerDeal{},
	StateKeyField:   "State",
	Events:          providerstates.ProviderEvents,
	StateEntryFuncs: providerstates.ProviderStateEntryFuncs,
	FinalityStates:  providerstates.ProviderFinalityStates,
}
