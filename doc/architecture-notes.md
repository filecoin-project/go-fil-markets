Following storing data doc https://lotu.sh/en+storing-data on how to make a deal, particularly the code flow of the `lotus client deal` command.

Lotus API `ClientStartDeal` in `node/impl/client/client.go`. We jump to the storage market API `ProposeStorageDeal`, `github.com/filecoin-project/go-fil-markets/storagemarket/impl/client.go`.

At this point we enter the client FSM, we jump ahead to the `StorageDealFundsEnsured` state to analyze the information flow between client and provider. The name of the state can be misleading because only in the `ClientStateEntryFuncs` do we learn that this state triggers the function in charge of actually sending the proposal: `ProposeDeal`, `github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates/client_states.go`.

`ProposeDeal`, then `WriteDealProposal` in `github.com/filecoin-project/go-fil-markets/storagemarket/network/deal_stream.go`. This function is important because we can verify here exactly what are we sending down the stream (the `Proposal` structure with its inner substructures). Conceptually it seem as a good anchor point to compare against the deal flows of the spec. The stream is a libp2p one connected to the Miner peer ID.

At this point we jump to the provider, this needs to be very clear in the doc, we are talking about two different processes in two different nodes in two different hosts, even though everything is very close in the code.

To find the other side of the pipe we trace the `DealProtocolID` that identifies the libp2p stream. We jump to the `SetDelegate` function. (All this doesn't need to be in the doc, it illustrates the price to pay to get this information, it is much simpler if we already delineate what happens _where_ instead of reverse engineering the flow as described here).

We find `(*libp2pStorageMarketNetwork).SetDelegate()` that sets the stream handler. We can trace that to the creation of the provider itself in the Lotus miner node, this is very important because it helps connecting both code bases, highlighting these 3 that seem relevant to the analysis.

```Go
			Override(new(storagemarket.StorageProvider), modules.StorageProvider),
			Override(new(storagemarket.StorageProviderNode), storageadapter.NewProviderNodeAdapter),
			Override(HandleDealsKey, modules.HandleDeals),
```

(I'm including the node adapter because it seems it's used throughout the Markets code base when we need access to the Lotus node. Is that the "environment" I've seen around? We should have a clear comment that this is the Lotus full node we are talking to.)

Back to the handlers, we go to `(*Provider).HandleDealStream()` which seems the entry point for the deal proposals the provider will receive from a client. In `receiveDeal()` we can see the start of the provider FSM, also of interest to be able to better trace the execution of both sides. We advance to the `StorageDealValidating` and `ValidateDealProposal`.

Both **client and provider seem to share states**, this is extremely confusing at first, even if the state are conceptually related in the deal flow of the spec, we can inadvertently jump from the provider to the client code without even realizing it (is there an algorithmic reason to share this constants?).

At this point we see a common Lotus API we can recognize: `GetChainHead` (I'm assuming someone reading this already is somewhat familiar with the general Lotus architecture so this kind of reference seems useful). We can see here the `environment.Node()` as just a proxy to the Lotus node created before. We verify (important) the signature here, also price and balance related checks.

In `DecideOnProposal` we actually return the deal acceptance to the client. At this point do we still use the same stream? In `SendSignedResponse` (`github.com/filecoin-project/go-fil-markets/storagemarket/impl/provider.go`), it would seem so, check `DealStream`.

How do we check this back in the client? Let's pick up from the last known state in `ProposeDeal`, we triggered `ClientEventDealProposed` that seems to take us to `StorageDealWaitingForDataRequest`, calling `WaitingForDataRequest()`. Is `ReadDealResponse()` just stuck on a read waiting for the provider to respond? We are unmarshaling a `SignedResponse` so it would seem so.

`StartDataTransfer()`: we open a _new_ channel/stream for the data (important). We trigger `ClientEventDataTransferInitiated` and then it's not clear how to continue. Following the events we reach `ClientDataTransferSubscriber` and `ClientEventDataTransferComplete`, that move the FSM to `StorageDealValidating`, but this was one of the initial states, it seems we are going back so this analysis is probably wrong.

We jump back to the provider code to see if it makes it easier to know where the flow is at. After `DecideOnProposal` we have the `ProviderEventDataRequested`, we fast forward to `ImportDataForDeal`, this is another point of information flow were we retrieve and store the data (not sure from where it was imported, the caller of this seems to be in Lotus), we verify here the CommP (important).

After that, `StorageDealEnsureProviderFunds`, but before note:

> Another thing that makes following the code even harder is that there doesn't seem to be a clear rule of which event has a function associated to it, we are forced to explicitly check `ProviderStateEntryFuncs` every time, which is very error prone especially in cases where the event has many references where it's harder to spot the function assignment. Not all states have a handler and there is no clear demarcation of when I'm missing a handler or not.

Continue with `EnsureProviderFunds()`, we seem to ensure our own funds (not critical), then `ProviderEventFundingInitiated`, `WaitForFunding()`, `ProviderEventFunded`, not sure how the self-check/fund works so skipping.

We hit `PublishDeal()` after the funding (on-chain, critical), most of the logic seems to be owned by Lotus in `PublishDeals()`. Aside, how does non-repudiation work here? We publish the initial offer, which was signed by the client (not the provider), but then the provider signs the Filecoin message that contains the deal offer with its signature, is that how we aggregate both partie's signatures?

Skipping the internal sealing, aside:

> Pretty please, let's structure state transitions in a _standard_ way so it's easier to quickly recognize in all cases the event, from and to. This is just a stylistic detail but since the reader needs to manually tie the states together it adds up to a measurable cognitive effort after a couple hours of studying this code.

I'm skipping `OnDealSectorCommitted()` to save time but I'm interested in understanding what _commit_ means in the context of markets, is that the same as the _commit_ from the miner actor?

Skipping internal record keeping in `RecordPieceInfo()`, it would seem this is the end of the provider deal flow.

Can we go back to the client with this information? What new has happened on the provider side? We received the data and published the deal, we try to find hints of that in the client. With that in mind we jump to `ValidateDealPublished()`, at this point we seem to already have the CID of the published deal (assuming it was sent by the provider at some point) and we specifically fetch and validate the message (important).
* Note for security: we explicitly check that the receiver is the `StorageMarketActorAddr`, is this a defensive check (which is fine, and even a good thing)? We should state that clearly in the code if so, otherwise this creates the erroneous perception that the `PublishStorageDeals` message can be sent to anyone (when the Actors VM should explicitly enforce this already).
* (Note to self: we first call `GetMessage()` directly on the store but only later `WaitForMessage()`, not sure I follow this, if the message is already in the store didn't we got it from the synced chain in the first place, or is there another path? Message pool in the miner? We are in the client side here though.)

We finally check `VerifyDealActivated()`, it similarly calls `OnDealSectorCommitted()` so it seem we do in fact explicitly check for the precommit call to succeed on chain (assuming the precommit is for the sector that holds our data), **this should point to the part of the spec that we are enforcing or referencing** (same for every step in the way before).

It seems to end on `StorageDealActive`, how do we actually now this is the end of the transitions? We seem to need to manually check there is no `To()` call for this state.
