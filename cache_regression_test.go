package utils

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestExpCacheExpiredCleanupKeepsFreshValue verifies that removing an expired ExpCache entry never deletes a
// value that was stored under the same key concurrently. Writers repeatedly re-store keys whose previous value
// has expired and read them straight back, while the background cleaner and concurrent readers remove expired
// entries; a miss for a value read back well within its ttl means a fresh value was deleted. It is a regression
// guard for the check-then-delete race in ExpCache.runClean and ExpCache.Load.
func TestExpCacheExpiredCleanupKeepsFreshValue(t *testing.T) {
	t.Parallel()
	const (
		ttl      = 200 * time.Microsecond
		writers  = 8
		keys     = 20
		duration = 2 * time.Second
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := NewExpCache[int](ctx, ttl)

	var (
		lost, samples atomic.Int64
		stop          atomic.Bool
		wg            sync.WaitGroup
	)
	deadline := time.Now().Add(duration)
	for w := range writers {
		names := make([]string, keys)
		for i := range names {
			names[i] = strconv.Itoa(w) + "-" + strconv.Itoa(i)
		}

		wg.Add(2)
		go func() { // writer: store, read back, and count fresh misses
			defer wg.Done()
			for !stop.Load() && time.Now().Before(deadline) {
				for _, name := range names {
					beforeStore := time.Now().UTC()
					cache.Store(name, w)
					got, ok := cache.Load(name)
					if time.Now().UTC().Sub(beforeStore) >= ttl/2 {
						continue // too slow to prove the value was still fresh
					}
					samples.Add(1)
					if !ok || got != w {
						lost.Add(1)
						stop.Store(true)
					}
				}
			}
		}()
		go func() { // reader: hit expired entries so Load takes its delete path
			defer wg.Done()
			for !stop.Load() && time.Now().Before(deadline) {
				for _, name := range names {
					cache.Load(name)
				}
			}
		}()
	}
	wg.Wait()

	t.Logf("fresh read-backs checked: %d, lost: %d", samples.Load(), lost.Load())
	require.Positive(t, samples.Load(), "no read-back completed within ttl/2; the check proved nothing")
	require.Zero(t, lost.Load(), "a value read back within its ttl was deleted by expired-entry cleanup")
}
