package storageimpl_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
)

func TestConfigure(t *testing.T) {
	p := &storageimpl.Provider{}

	assert.False(t, p.UniversalRetrievalEnabled())

	p.Configure(
		storageimpl.EnableUniversalRetrieval(),
	)

	assert.True(t, p.UniversalRetrievalEnabled())
}
