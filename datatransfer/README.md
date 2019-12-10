# How To Use go-fil-components/datatransfer

## Initialize a data transfer module
```go
    package mypackage

    // You need the following imports:
    import (
    	gsimpl "github.com/ipfs/go-graphsync/impl"
        dtgs "github.com/filecoin-project/go-fil-components/datatransfer"
    	"github.com/libp2p/go-libp2p-core/host"
)
    // . . .
    // You will need to set up a libp2p host.Host and a go-graphsync GraphExchange.
    // then you can create a new instance of GraphsyncDataTransfer with:
    func MyFunc(h host.Host, gs graphsync.GraphExchange) {
        datatransfer := dtgs.NewGraphSyncDataTransfer(h, gs)
    }
```
## open a push/pull request
## Subscribe to events
## Register a validator