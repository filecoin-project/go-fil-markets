package testing

import (
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type ReadBlockResponse struct {
	Block retrievalmarket.Block
	Done  bool
	Err   error
}
