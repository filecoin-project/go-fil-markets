package discoveryimpl

import (
	"github.com/filecoin-project/boost-gfm/discovery"
)

func Multi(r discovery.PeerResolver) discovery.PeerResolver { // TODO: actually support multiple mechanisms
	return r
}
