package testharness

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/go-fil-markets/discovery"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

// RetrievalClientHarness has a reference to a retrieval client and its
// component parts
type RetrievalClientHarness struct {
	Client          retrievalmarket.RetrievalClient
	ClientNode      *testnodes.TestRetrievalClientNode
	ExpectedVoucher *paych.SignedVoucher
	Paych           address.Address
	MultiStore      *multistore.MultiStore
}

type RetrievalClientParams struct {
	Host                   host.Host
	Dstore                 datastore.Datastore
	MultiStore             *multistore.MultiStore
	DataTransfer           datatransfer.Manager
	PeerResolver           discovery.PeerResolver
	RetrievalStoredCounter *storedcounter.StoredCounter
}

// NewRetrievalClient creates a retrieval client from the underlying components
// passed in the params
func NewRetrievalClient(t *testing.T, params *RetrievalClientParams) *RetrievalClientHarness {
	// Set up a payment channel
	cids := shared_testutil.GenerateCids(2)
	clientPaymentChannel, err := address.NewActorAddress([]byte("a"))

	paymentChannelRecorder := func(client, miner address.Address, amt abi.TokenAmount) {}
	laneRecorder := func(paymentChannel address.Address) {}
	paymentVoucherRecorder := func(v *paych.SignedVoucher) {}

	// Create a client node
	expectedVoucher := shared_testutil.MakeTestSignedVoucher()
	require.NoError(t, err)
	clientNode := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		Lane:                   expectedVoucher.Lane,
		PayCh:                  clientPaymentChannel,
		Voucher:                expectedVoucher,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,
		CreatePaychCID:         cids[0],
		AddFundsCID:            cids[1],
		IntegrationTest:        true,
	})

	// Create a libp2p node on the host and a datastore
	nw1 := network.NewFromLibp2pHost(params.Host, network.RetryParameters(0, 0, 0, 0))
	clientDs := namespace.Wrap(params.Dstore, datastore.NewKey("/retrievals/client"+fmt.Sprintf("%d", rand.Uint64())))

	// Create a retrieval client
	client, err := retrievalimpl.NewClient(nw1, params.MultiStore, params.DataTransfer, clientNode, params.PeerResolver, clientDs, params.RetrievalStoredCounter)
	require.NoError(t, err)

	return &RetrievalClientHarness{
		Client:          client,
		ClientNode:      clientNode,
		ExpectedVoucher: expectedVoucher,
		Paych:           clientPaymentChannel,
		MultiStore:      params.MultiStore,
	}
}

// RetrievalProviderHarness has a reference to a retrieval provider and its
// component parts
type RetrievalProviderHarness struct {
	Provider     retrievalmarket.RetrievalProvider
	ProviderNode *testnodes.TestRetrievalProviderNode
	PieceStore   *shared_testutil.TestPieceStore
	DataTransfer datatransfer.Manager
	NodeDeps     *shared_testutil.Libp2pNodeDeps
}

type RetrievalProviderParams struct {
	MockNet      mocknet.Mocknet
	PaymentAddr  address.Address
	DataTransfer datatransfer.Manager
	NodeDeps     *shared_testutil.Libp2pNodeDeps
	AskParams    retrievalmarket.Params
}

// NewRetrievalClient creates a retrieval provider from the underlying
// components passed in the params
func NewRetrievalProvider(t *testing.T, params *RetrievalProviderParams) *RetrievalProviderHarness {
	h := &RetrievalProviderHarness{
		ProviderNode: testnodes.NewTestRetrievalProviderNode(),
		PieceStore:   shared_testutil.NewTestPieceStore(),
		DataTransfer: params.DataTransfer,
		NodeDeps:     params.NodeDeps,
	}

	// Create a libp2p node on the host and a datastore
	nw2 := network.NewFromLibp2pHost(params.NodeDeps.Host, network.RetryParameters(0, 0, 0, 0))
	providerDs := namespace.Wrap(params.NodeDeps.Dstore, datastore.NewKey("/retrievals/provider"))

	// Create a retrieval provider
	provider, err := retrievalimpl.NewProvider(params.PaymentAddr, h.ProviderNode, nw2, h.PieceStore, params.NodeDeps.MultiStore, params.DataTransfer, providerDs)
	require.NoError(t, err)

	// Set the provider's ask
	ask := provider.GetAsk()
	ask.PaymentInterval = params.AskParams.PaymentInterval
	ask.PaymentIntervalIncrease = params.AskParams.PaymentIntervalIncrease
	ask.PricePerByte = params.AskParams.PricePerByte
	provider.SetAsk(ask)

	h.Provider = provider
	return h
}

// MockUnseal clears out the provider's blockstore and sets it up to unseal
// the given CAR file into the blockstore
func (h *RetrievalProviderHarness) MockUnseal(ctx context.Context, t *testing.T, payloadCID cid.Cid, carData []byte) {
	sectorID := abi.SectorNumber(rand.Uint64())
	offset := abi.PaddedPieceSize(1000)
	pieceInfo := piecestore.PieceInfo{
		PieceCID: shared_testutil.GenerateCids(1)[0],
		Deals: []piecestore.DealInfo{
			{
				SectorID: sectorID,
				Offset:   offset,
				Length:   abi.UnpaddedPieceSize(uint64(len(carData))).Padded(),
			},
		},
	}
	h.ProviderNode.ExpectUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(uint64(len(carData))), carData)

	// Clear out provider blockstore - we want the provider to see that the
	// deal data is not in the blockstore, so it has to unseal the data first
	// and then place it in the blockstore for retrieval
	allCids, err := h.NodeDeps.Bstore.AllKeysChan(ctx)
	require.NoError(t, err)
	for c := range allCids {
		err = h.NodeDeps.Bstore.DeleteBlock(c)
		require.NoError(t, err)
	}

	expectedPiece := shared_testutil.GenerateCids(1)[0]
	cidInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPiece,
			},
		},
	}
	h.PieceStore.ExpectCID(payloadCID, cidInfo)
	h.PieceStore.ExpectPiece(expectedPiece, pieceInfo)
}
