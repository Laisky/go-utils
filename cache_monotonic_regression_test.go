package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// hasMonotonicReading reports whether t carries Go's monotonic clock reading.
// Round(0) strips it, so the two values differ exactly when one is present.
func hasMonotonicReading(t time.Time) bool {
	return t != t.Round(0)
}

// TestExpCachesUseMonotonicExpiry verifies that ExpCache and
// SingleItemExpCache compute expiry with the monotonic clock. Calling UTC()
// strips the monotonic reading, so expiry then followed the wall clock and a
// system clock jump (NTP step, manual change) could expire entries early or
// keep them far past their ttl.
func TestExpCachesUseMonotonicExpiry(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := NewExpCache[int](ctx, time.Minute)
	exp.Store("k", 1)
	item, ok := exp.data.Load("k")
	require.True(t, ok)
	require.True(t, hasMonotonicReading(item.(*expCacheItem).exp), "ExpCache expiry lost the monotonic reading")

	single := NewSingleItemExpCache[int](time.Minute)
	single.Set(1)
	require.True(t, hasMonotonicReading(single.expiredAt), "SingleItemExpCache expiry lost the monotonic reading")
}
