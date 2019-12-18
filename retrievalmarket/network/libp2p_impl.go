package network

import (
	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-core/host"
)

var log = logging.Logger("retrieval_network")

func NewFromLibp2pHost(host.Host) RetrievalMarketNetwork {
	return nil
}
// libp2pDataTransferNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
type libp2pDataTransferNetwork struct {
	host host.Host
	// inbound messages from the network are forwarded to the receiver
	receiver
}