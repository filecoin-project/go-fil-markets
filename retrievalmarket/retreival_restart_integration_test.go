package retrievalmarket_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-data-transfer/channelmonitor"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-log"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/stretchr/testify/require"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

// TODO
// TEST CONNECTION BOUNCE FOR ALL MEANINGFUL STATES OF THE CLIENT AND PROVIDER DEAL LIFECYCLE.
// CURRENTLY, WE ONLY TEST THIS FOR THE DEALSTATUS ONGOING STATE.

// TestBounceConnectionDealTransferOngoing tests that when the the connection is
// broken and then restarted during deal data transfer for an ongoing deal, the data transfer will resume and the deal will
// complete successfully.
func TestBounceConnectionDealTransferOngoing(t *testing.T) {
	bgCtx := context.Background()
	log.SetLogLevel("dt-impl", "debug")

	tcs := map[string]struct {
		unSealPrice             abi.TokenAmount
		pricePerByte            abi.TokenAmount
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		voucherAmts             []abi.TokenAmount
	}{
		"non-zero unseal, non zero prices per byte": {
			unSealPrice:             abi.NewTokenAmount(1000),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(1000), abi.NewTokenAmount(10136000), abi.NewTokenAmount(7736000)},
		},

		"zero unseal, non-zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(10136000), abi.NewTokenAmount(9784000)},
		},

		// TODO These cases are flaky -> some problem in data-transfer -> Graphsync
		"zero unseal, zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            big.Zero(),
			paymentInterval:         uint64(0),
			paymentIntervalIncrease: uint64(0),
			voucherAmts:             nil,
		},

		"non-zero unseal, zero price per byte": {
			unSealPrice:  abi.NewTokenAmount(1000),
			pricePerByte: big.Zero(),
			voucherAmts:  []abi.TokenAmount{abi.NewTokenAmount(1000)},
		},

		// TODO : Repeated Partial Payments
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dtClientNetRetry := dtnet.RetryParameters(time.Second, time.Second, 5, 1)
			restartConf := dtimpl.ChannelRestartConfig(channelmonitor.Config{
				MonitorPullChannels:    true,
				AcceptTimeout:          100 * time.Millisecond,
				Interval:               100 * time.Millisecond,
				MinBytesTransferred:    1,
				ChecksPerInterval:      10,
				RestartBackoff:         100 * time.Minute,
				MaxConsecutiveRestarts: 5,
				CompleteTimeout:        1000000000 * time.Millisecond,
			})
			td := shared_testutil.NewLibp2pTestData(bgCtx, t)
			td.DTNet1 = dtnet.NewFromLibp2pHost(td.Host1, dtClientNetRetry)
			depGen := dependencies.NewDepGenerator()
			depGen.ClientNewDataTransfer = func(ds datastore.Batching, dir string, transferNetwork dtnet.DataTransferNetwork, transport datatransfer.Transport) (datatransfer.Manager, error) {
				return dtimpl.NewDataTransfer(ds, dir, transferNetwork, transport, restartConf)
			}
			deps := depGen.New(t, bgCtx, td, testnodes.NewStorageMarketState(), "", noOpDelay, noOpDelay)

			sh := testharness.NewHarnessWithTestData(t, td, deps, true, false)

			// do a storage deal
			storageClientSeenDeal := doStorage(t, bgCtx, sh)
			ctxTimeout, canc := context.WithTimeout(bgCtx, 25*time.Second)
			defer canc()

			// create a retrieval test harness
			rh := newRetrievalHarness(ctxTimeout, t, sh, storageClientSeenDeal, retrievalmarket.Params{
				UnsealPrice:             tc.unSealPrice,
				PricePerByte:            tc.pricePerByte,
				PaymentInterval:         tc.paymentInterval,
				PaymentIntervalIncrease: tc.paymentIntervalIncrease,
			})
			clientHost := rh.TestDataNet.Host1.ID()
			providerHost := rh.TestDataNet.Host2.ID()

			// Bounce connection after this many bytes have been queued for sending
			bounceConnectionAt := map[uint64]bool{
				1000: false,
				//3000: false,
				//5000: false,
				//7000: false,
				//9000: false,
			}

			sh.DTProvider.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
				if event.Code == datatransfer.DataQueuedProgress {
					// Check if enough bytes have been queued that the connection
					// should be bounced
					for at, already := range bounceConnectionAt {
						if channelState.Sent() > at && !already {
							bounceConnectionAt[at] = true

							// Break the connection
							sent := channelState.Sent()
							t.Logf("breaking connection after sending %d bytes", sent)
							rh.TestDataNet.MockNet.DisconnectPeers(clientHost, providerHost)
							rh.TestDataNet.MockNet.UnlinkPeers(clientHost, providerHost)

							go func() {
								t.Logf("restoring connection after sending %d bytes", sent)
								time.Sleep(100 * time.Millisecond)
								rh.TestDataNet.MockNet.LinkPeers(clientHost, providerHost)
							}()
						}
					}
				}
			})

			// Subscribe to client events & wait for the terminal DealStatusCompleted state.
			clientDealStateChan := make(chan retrievalmarket.ClientDealState)
			rh.Client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted:
					clientDealStateChan <- state
				default:
					msg := `
						Client:
						Event:           %s
						Status:          %s
						TotalReceived:   %d
						BytesPaidFor:    %d
						CurrentInterval: %d
						TotalFunds:      %s
						Message:         %s
						`

					fmt.Printf(msg, retrievalmarket.ClientEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
						state.TotalFunds.String(), state.Message)
				}
			})

			// Subscribe to provider events & wait for the terminal DealStatusCompleted state.
			providerDealStateChan := make(chan retrievalmarket.ProviderDealState)
			rh.Provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted:
					providerDealStateChan <- state
				default:
					msg := `
					Provider:
					Event:           %s
					Status:          %s
					TotalSent:       %d
					FundsReceived:   %s
					Message:		 %s
					CurrentInterval: %d
					`
					fmt.Printf(msg, retrievalmarket.ProviderEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
						state.CurrentInterval)
				}
			})

			// Retrieve file.
			fsize, clientStoreID := doRetrieve(t, bgCtx, rh, sh, tc.voucherAmts)

			// Wait for both the client & provider to see the deal completion and then assert.
			ctxTimeout, cancel := context.WithTimeout(bgCtx, 60*time.Second)
			defer cancel()

			// verify that client subscribers will be notified of state changes
			var clientDealState retrievalmarket.ClientDealState
			select {
			case <-ctxTimeout.Done():
				t.Error("deal never completed")
				t.FailNow()
			case clientDealState = <-clientDealStateChan:
			}

			ctxTimeout, cancel = context.WithTimeout(bgCtx, 60*time.Second)
			defer cancel()
			var providerDealState retrievalmarket.ProviderDealState
			select {
			case <-ctxTimeout.Done():
				t.Error("provider never saw completed deal")
				t.FailNow()
			case providerDealState = <-providerDealStateChan:
			}

			require.Equal(t, retrievalmarket.DealStatusCompleted, providerDealState.Status)
			require.Equal(t, retrievalmarket.DealStatusCompleted, clientDealState.Status)
			rh.ClientNode.VerifyExpectations(t)
			sh.TestData.VerifyFileTransferredIntoStore(t, cidlink.Link{Cid: sh.PayloadCid}, clientStoreID, false, uint64(fsize))
		})
	}
}
