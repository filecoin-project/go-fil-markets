package test_harnesses

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/pieceio/cario"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

type ProviderHarness struct {
	ctx          context.Context
	t            *testing.T
	Filename     string
	Filesize     uint64
	ExpectedQR   *retrievalmarket.QueryResponse
	Network      network.RetrievalMarketNetwork
	PayloadCID   cid.Cid
	PaychAddr    address.Address
	PieceInfo    *piecestore.PieceInfo
	PieceLink    ipld.Link
	PieceStore   piecestore.PieceStore
	ProviderNode *testnodes.TestRetrievalProviderNode
	retrievalmarket.RetrievalProvider
	TestData *shared_testutil.Libp2pTestData
}

func (ph *ProviderHarness) Bootstrap(ctx context.Context, t *testing.T, filename string,
	filesize uint64, unsealing bool) *ProviderHarness {
	ph.ctx = ctx
	ph.t = t
	ph.Filename = filename
	ph.Filesize = filesize
	var err error
	if ph.TestData == nil {
		ph.TestData = shared_testutil.NewLibp2pTestData(ph.ctx, t)
	}
	if ph.Network == nil {
		ph.Network = network.NewFromLibp2pHost(ph.TestData.Host2)
	}
	if ph.PaychAddr == address.Undef {
		ph.PaychAddr, err = address.NewIDAddress(999)
		require.NoError(ph.t, err)
	}

	// Inject a unixFS file on the provider side to its blockstore
	// obtained via `ls -laf` on this file
	fpath := filepath.Join("retrievalmarket", "impl", "fixtures", filename)

	ph.PieceLink = ph.TestData.LoadUnixFSFile(t, fpath, true)
	c, ok := ph.PieceLink.(cidlink.Link)
	require.True(t, ok)
	ph.PayloadCID = c.Cid
	providerPaymentAddr, err := address.NewIDAddress(575)
	require.NoError(t, err)
	paymentInterval := uint64(10000)
	paymentIntervalIncrease := uint64(1000)
	pricePerByte := abi.NewTokenAmount(1000)

	ph.ExpectedQR = &retrievalmarket.QueryResponse{
		Size:                       1024,
		PaymentAddress:             providerPaymentAddr,
		MinPricePerByte:            pricePerByte,
		MaxPaymentInterval:         paymentInterval,
		MaxPaymentIntervalIncrease: paymentIntervalIncrease,
	}

	if ph.ProviderNode == nil {
		ph.ProviderNode = testnodes.NewTestRetrievalProviderNode()
	}

	pieceInfo := ph.setupUnseal(unsealing)
	ps := shared_testutil.NewTestPieceStore()
	expectedPiece := shared_testutil.GenerateCids(1)[0]
	cidInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPiece,
			},
		},
	}
	ps.ExpectCID(ph.PayloadCID, cidInfo)
	ps.ExpectPiece(expectedPiece, pieceInfo)

	provider, err := retrievalimpl.NewProvider(providerPaymentAddr, ph.ProviderNode, ph.Network,
		ps, ph.TestData.Bs2, ph.TestData.Ds2)
	require.NoError(t, err)

	provider.SetPaymentInterval(ph.ExpectedQR.MaxPaymentInterval,
		ph.ExpectedQR.MaxPaymentIntervalIncrease)
	provider.SetPricePerByte(ph.ExpectedQR.MinPricePerByte)
	require.NoError(t, provider.Start())
	ph.RetrievalProvider = provider
	ph.PieceStore = ps
	return ph
}
func (ph *ProviderHarness) setupUnseal(unsealing bool) (pi piecestore.PieceInfo) {
	if !unsealing {
		pi = piecestore.PieceInfo{
			Deals: []piecestore.DealInfo{
				{
					Length: ph.ExpectedQR.Size,
				},
			},
		}
		return pi
	}

	cio := cario.NewCarIO()
	var buf bytes.Buffer
	err := cio.WriteCar(ph.ctx, ph.TestData.Bs2, ph.PayloadCID, shared.AllSelector(), &buf)
	require.NoError(ph.t, err)
	carData := buf.Bytes()
	sectorID := uint64(100000)
	offset := uint64(1000)
	pi = piecestore.PieceInfo{
		Deals: []piecestore.DealInfo{
			{
				SectorID: sectorID,
				Offset:   offset,
				Length:   uint64(len(carData)),
			},
		},
	}
	ph.ProviderNode.ExpectUnseal(sectorID, offset, uint64(len(carData)), carData)
	// clear out provider blockstore
	allCids, err := ph.TestData.Bs2.AllKeysChan(ph.ctx)
	require.NoError(ph.t, err)
	for c := range allCids {
		err = ph.TestData.Bs2.DeleteBlock(c)
		require.NoError(ph.t, err)
	}

	return pi
}
