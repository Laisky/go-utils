package utils

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Behavior-first contract for the proposed additive limiter. allowAt supplies
// deterministic time and is intentionally package-private.
func (l *KeyedRateLimiter[K]) allowAt(key K, now time.Time) bool {
	if !validKey(key, l.checkDynamicKey) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowLocked(key, now)
}

func config() KeyedRateLimiterConfig {
	return KeyedRateLimiterConfig{TotalPerSecond: 1, TotalBurst: 10, KeyPerSecond: 1, KeyBurst: 2, MaxKeys: 3}
}

func TestKeyedConstructorValidation(t *testing.T) {
	good := config()
	tests := []struct {
		name string
		edit func(*KeyedRateLimiterConfig)
	}{
		{"zero global rate", func(c *KeyedRateLimiterConfig) { c.TotalPerSecond = 0 }},
		{"negative key rate", func(c *KeyedRateLimiterConfig) { c.KeyPerSecond = -1 }},
		{"zero global burst", func(c *KeyedRateLimiterConfig) { c.TotalBurst = 0 }},
		{"negative key burst", func(c *KeyedRateLimiterConfig) { c.KeyBurst = -1 }},
		{"no key capacity", func(c *KeyedRateLimiterConfig) { c.MaxKeys = 0 }},
		{"rate overflow", func(c *KeyedRateLimiterConfig) { c.TotalPerSecond = 1_000_000_001 }},
		{"burst overflow", func(c *KeyedRateLimiterConfig) { c.KeyBurst = math.MaxInt }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := good
			tt.edit(&c)
			_, err := NewKeyedRateLimiter[string](c)
			require.Error(t, err)
		})
	}
	_, err := NewKeyedRateLimiter[string](good)
	require.NoError(t, err)
}

func TestKeyedRefusalDoesNotSpendOtherBudget(t *testing.T) {
	now := time.Unix(100, 0)
	cfg := config()
	cfg.TotalBurst = 2
	cfg.KeyBurst = 1
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	require.True(t, l.allowAt("a", now))
	require.False(t, l.allowAt("a", now)) // Key empty; global remains available.
	require.True(t, l.allowAt("b", now))
	require.False(t, l.allowAt("c", now)) // Total empty; no c entry created.
	require.Len(t, l.entries, 2)
	require.True(t, l.allowAt("c", now.Add(time.Second)))
}

func TestKeyedRefillBoundaryAndBackwardClock(t *testing.T) {
	cfg := config()
	cfg.TotalPerSecond = 3
	cfg.KeyPerSecond = 3
	cfg.TotalBurst = 1
	cfg.KeyBurst = 1
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.False(t, l.allowAt("a", t0))
	require.False(t, l.allowAt("a", t0.Add(333_333_333*time.Nanosecond)))
	require.False(t, l.allowAt("a", t0.Add(-time.Second)))
	require.True(t, l.allowAt("a", t0.Add(333_333_334*time.Nanosecond)))
	require.False(t, l.allowAt("a", t0.Add(333_333_334*time.Nanosecond)))
}

func TestKeyedCapacityNeverEvictsDepletedBudget(t *testing.T) {
	cfg := config()
	cfg.MaxKeys = 1
	cfg.KeyBurst = 2
	cfg.TotalBurst = 10
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.False(t, l.allowAt("b", t0)) // Cannot discard a's partial bucket.
	require.True(t, l.allowAt("a", t0))  // Capacity refusal spent nothing.
	require.False(t, l.allowAt("b", t0.Add(time.Second)))
	require.True(t, l.allowAt("b", t0.Add(2*time.Second)))
	require.Len(t, l.entries, 1)
}

func TestKeyedEvictionRespectsRecentTake(t *testing.T) {
	cfg := config()
	cfg.MaxKeys = 1
	cfg.KeyBurst = 2
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.True(t, l.allowAt("a", t0.Add(time.Second)))
	require.False(t, l.allowAt("b", t0.Add(time.Second)))
	require.True(t, l.allowAt("b", t0.Add(2*time.Second)))
}

func TestKeyedConcurrentAdmissionsAreAtomic(t *testing.T) {
	cfg := config()
	cfg.TotalBurst = 37
	cfg.KeyBurst = 100
	cfg.MaxKeys = 1
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	var n atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.allowAt("same", t0) {
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 37, n.Load())
}

func TestKeyedNumericLimitsAndNoNanosecondOverflow(t *testing.T) {
	cfg := config()
	cfg.TotalBurst = 1_000_000_000
	cfg.KeyBurst = 1_000_000_000
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.True(t, l.allowAt("a", t0))
	require.EqualValues(t, 999_999_998_000_000_000, l.total.credit)
	require.True(t, l.allowAt("a", t0.Add(time.Duration(math.MaxInt64))))
	require.EqualValues(t, 999_999_999_000_000_000, l.total.credit)
}

func TestKeyedZeroTimeAndInvalidKeys(t *testing.T) {
	cfg := config()
	cfg.KeyBurst = 1
	l, err := NewKeyedRateLimiter[any](cfg)
	require.NoError(t, err)
	zero := time.Time{}
	require.True(t, l.allowAt("a", zero))
	require.False(t, l.allowAt("a", zero))
	require.True(t, l.allowAt("a", zero.Add(time.Second)))
	require.False(t, l.allowAt(math.NaN(), zero))
	require.False(t, l.allowAt([]int{1}, zero))
	require.False(t, l.allowAt(struct{ Value any }{Value: []int{1}}, zero))
	require.False(t, l.allowAt(struct{ Value any }{Value: math.NaN()}, zero))
	require.Len(t, l.entries, 1)
	structLimiter, err := NewKeyedRateLimiter[struct{ Value any }](cfg)
	require.NoError(t, err)
	require.False(t, structLimiter.allowAt(struct{ Value any }{Value: []int{1}}, zero))
	require.False(t, structLimiter.allowAt(struct{ Value any }{Value: math.NaN()}, zero))
}

func TestKeyedConcurrentGlobalAcrossKeys(t *testing.T) {
	cfg := config()
	cfg.TotalBurst, cfg.KeyBurst, cfg.MaxKeys = 37, 100, 1000
	l, err := NewKeyedRateLimiter[int](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(key int) {
			defer wg.Done()
			if l.allowAt(key, t0) {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 37, admitted.Load())
	require.Len(t, l.entries, 37)
}

func TestKeyedHeapTracksUpdatedKeys(t *testing.T) {
	cfg := config()
	cfg.MaxKeys = 3
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	for _, key := range []string{"a", "b", "c"} {
		require.True(t, l.allowAt(key, t0))
	}
	require.True(t, l.allowAt("a", t0.Add(500*time.Millisecond)))
	require.True(t, l.allowAt("d", t0.Add(time.Second)))
	require.Contains(t, l.entries, "a")
	require.Contains(t, l.entries, "d")
	require.Len(t, l.entries, 3)
	for i, entry := range l.expiry {
		require.Equal(t, i, entry.index)
		if i > 0 {
			require.False(t, entry.fullAt.Before(l.expiry[(i-1)/2].fullAt))
		}
	}
}

func TestKeyedGlobalRefusalDoesNotEvict(t *testing.T) {
	cfg := config()
	cfg.MaxKeys, cfg.TotalBurst, cfg.KeyBurst, cfg.KeyPerSecond = 2, 2, 1, 2
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.True(t, l.allowAt("b", t0))
	require.False(t, l.allowAt("c", t0.Add(500*time.Millisecond)))
	require.Contains(t, l.entries, "a")
	require.Contains(t, l.entries, "b")
	require.NotContains(t, l.entries, "c")
	require.Len(t, l.entries, 2)
}

func TestKeyedHighestRateAndBurst(t *testing.T) {
	cfg := config()
	cfg.TotalPerSecond, cfg.KeyPerSecond = 1_000_000_000, 1_000_000_000
	cfg.TotalBurst, cfg.KeyBurst = 1_000_000_000, 1_000_000_000
	l, err := NewKeyedRateLimiter[string](cfg)
	require.NoError(t, err)
	t0 := time.Unix(100, 0)
	require.True(t, l.allowAt("a", t0))
	require.True(t, l.allowAt("a", t0))
	require.EqualValues(t, 999_999_998_000_000_000, l.total.credit)
	require.True(t, l.allowAt("a", t0.Add(time.Nanosecond)))
	require.EqualValues(t, 999_999_998_000_000_000, l.total.credit)
}
