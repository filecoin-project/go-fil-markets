package carstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadOnlyStoreTracker(t *testing.T) {
	k1 := "k1"
	tracker := NewReadOnlyStoreTracker()

	_, err := tracker.Get(k1)
	require.True(t, IsNotFound(err))
}
