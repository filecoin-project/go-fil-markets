package shared

import (
	"github.com/ipld/go-ipld-prime"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
)

// AllSelector is a compatibility alias for an entire DAG non-matching-selector
// Use selectorparse.CommonSelector_ExploreAllRecursively directly in new code
func AllSelector() ipld.Node { return selectorparse.CommonSelector_ExploreAllRecursively }
