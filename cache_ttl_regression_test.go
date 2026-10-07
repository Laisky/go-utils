package utils

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ttlCacheLen counts the entries currently held by c, expired or not.
func ttlCacheLen[T any](c *TtlCache[T]) int {
	n := 0
	c.kv.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// TestTtlCacheEvictsExpiredEntriesWithoutReads verifies that TtlCache releases
// expired entries in the background even when they are never read again.
// Previously the cleaner only pruned its skip-list index and left every value
// in the map until a Get happened to touch it, so write-once keys leaked.
func TestTtlCacheEvictsExpiredEntriesWithoutReads(t *testing.T) {
	t.Parallel()
	c := NewTtlCache[int]()
	defer c.Close()

	for i := range 500 {
		c.Set("k"+strconv.Itoa(i), i, 10*time.Millisecond)
	}
	c.Set("fresh", 1, time.Hour)

	require.Eventually(t, func() bool { return ttlCacheLen(c) == 1 },
		10*time.Second, 20*time.Millisecond, "expired entries were never evicted; %d remain", ttlCacheLen(c))
	v, ok := c.Get("fresh")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

// TestTtlCacheExpiredReadKeepsFreshValue verifies that a Get which observes an
// expired entry never deletes a fresh value stored under the same key
// concurrently (check-then-delete race).
func TestTtlCacheExpiredReadKeepsFreshValue(t *testing.T) {
	t.Parallel()
	const (
		ttl      = 200 * time.Microsecond
		writers  = 8
		keys     = 20
		duration = 2 * time.Second
	)
	c := NewTtlCache[int]()
	defer c.Close()

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
		go func() {
			defer wg.Done()
			for !stop.Load() && time.Now().Before(deadline) {
				for _, name := range names {
					start := time.Now()
					c.Set(name, w, ttl)
					got, ok := c.Get(name)
					if time.Since(start) >= ttl/2 {
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
		go func() {
			defer wg.Done()
			for !stop.Load() && time.Now().Before(deadline) {
				for _, name := range names {
					c.Get(name)
				}
			}
		}()
	}
	wg.Wait()

	t.Logf("fresh read-backs checked: %d, lost: %d", samples.Load(), lost.Load())
	require.Positive(t, samples.Load(), "no read-back completed within ttl/2; the check proved nothing")
	require.Zero(t, lost.Load(), "a fresh value was deleted by an expired read")
}
