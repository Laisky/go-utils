package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============================================================
// Behavioral Tests – Token refill accuracy
// ============================================================

func TestRateLimiterRefillAccuracy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("low rate: refill 3 per sec", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 3, Max: 30},
			WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		// Wait 2 seconds, expect ~6 tokens (allow ±1 for timing)
		time.Sleep(2050 * time.Millisecond)
		tokens := rl.Len()
		require.InDelta(t, 6, tokens, 1,
			"expected ~6 tokens after 2s at 3/s, got %d", tokens)
	})

	t.Run("medium rate: refill 100 per sec", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 100, Max: 1000},
			WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		time.Sleep(1050 * time.Millisecond)
		tokens := rl.Len()
		require.InDelta(t, 100, tokens, 15,
			"expected ~100 tokens after 1s at 100/s, got %d", tokens)
	})

	t.Run("high rate: refill 50000 per sec", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 50000, Max: 200000},
			WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		time.Sleep(1050 * time.Millisecond)
		tokens := rl.Len()
		require.InDelta(t, 50000, tokens, 5000,
			"expected ~50000 tokens after 1s at 50000/s, got %d", tokens)
	})
}

// TestRateLimiterMaxCap verifies tokens never exceed Max even after long refill periods.
func TestRateLimiterMaxCap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 10, Max: 15},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	// Wait long enough for tokens to far exceed Max if uncapped
	time.Sleep(3050 * time.Millisecond)
	tokens := rl.Len()
	require.LessOrEqual(t, tokens, 15,
		"tokens %d should never exceed Max=15", tokens)
	require.GreaterOrEqual(t, tokens, 13,
		"tokens %d should be near Max=15 after 3s", tokens)
}

// ============================================================
// Behavioral Tests – Sustained throughput over time
// ============================================================

func TestRateLimiterSustainedThroughput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Use tier2 (NPerSec>10) for smoother refill (100ms interval) so that
	// sustained throughput measurement is more accurate.
	const nPerSec = 50
	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: nPerSec, Max: nPerSec * 10},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	// Continuously consume tokens for 3 seconds and count total allowed
	var allowed int64
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rl.Allow() {
			allowed++
		}
		time.Sleep(time.Millisecond)
	}

	// Over 3 seconds at 50/s, expect ~150 allowed
	expected := int64(nPerSec * 3)
	require.InDelta(t, expected, allowed, float64(expected)*0.15,
		"sustained throughput should be ~%d over 3s at %d/s, got %d", expected, nPerSec, allowed)
}

// ============================================================
// Behavioral Tests – Burst and recovery
// ============================================================

func TestRateLimiterBurstAndRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 10, Max: 20},
		WithAvailableTokens(20))
	require.NoError(t, err)
	defer rl.Close()

	// Burst: consume all 20 tokens immediately
	for i := 0; i < 20; i++ {
		require.True(t, rl.Allow(), "burst token %d should be allowed", i)
	}
	require.False(t, rl.Allow(), "should be exhausted after burst")

	// Recovery: wait 1 second, should get ~10 tokens back
	time.Sleep(1050 * time.Millisecond)
	tokens := rl.Len()
	require.InDelta(t, 10, tokens, 2,
		"should recover ~10 tokens after 1s, got %d", tokens)

	// Can burst again with recovered tokens
	consumed := 0
	for rl.Allow() {
		consumed++
	}
	require.InDelta(t, 10, consumed, 2,
		"should consume ~10 recovered tokens, got %d", consumed)
}

// ============================================================
// Behavioral Tests – Concurrent access correctness
// ============================================================

func TestRateLimiterConcurrentAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const maxTokens = 1000
	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 100, Max: maxTokens},
		WithAvailableTokens(maxTokens))
	require.NoError(t, err)
	defer rl.Close()

	// Launch many goroutines all competing to consume tokens
	const numGoroutines = 50
	var totalAllowed atomic.Int64
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if rl.Allow() {
					totalAllowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	// Total allowed should not exceed initial tokens + whatever was refilled
	// during the test. With 1000 initial tokens and a very brief test,
	// we mainly check no over-consumption happened.
	allowed := totalAllowed.Load()
	t.Logf("concurrent test: %d/%d requests allowed", allowed, numGoroutines*100)
	require.LessOrEqual(t, allowed, int64(maxTokens+200),
		"allowed %d should not wildly exceed initial tokens + minor refill", allowed)
	require.Greater(t, allowed, int64(0), "some requests should have been allowed")
}

func TestRateLimiterConcurrentAllowN(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 50, Max: 500},
		WithAvailableTokens(500))
	require.NoError(t, err)
	defer rl.Close()

	// Concurrently consume with varying AllowN sizes
	var totalConsumed atomic.Int64
	var wg sync.WaitGroup

	for _, n := range []int{1, 2, 5, 10, 3, 7} {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if rl.AllowN(n) {
					totalConsumed.Add(int64(n))
				}
			}
		}()
	}
	wg.Wait()

	consumed := totalConsumed.Load()
	t.Logf("concurrent AllowN: consumed %d tokens", consumed)
	// Consumed should not exceed initial + refill
	require.LessOrEqual(t, consumed, int64(600),
		"consumed %d should not wildly exceed 500 initial + refill", consumed)
}

// ============================================================
// Behavioral Tests – Token count never goes negative
// ============================================================

func TestRateLimiterTokensNeverNegative(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 5, Max: 10},
		WithAvailableTokens(1))
	require.NoError(t, err)
	defer rl.Close()

	// Aggressively consume
	for i := 0; i < 100; i++ {
		rl.AllowN(3)
		tokens := rl.Len()
		require.GreaterOrEqual(t, tokens, 0,
			"tokens should never go negative, got %d at iteration %d", tokens, i)
	}
}

// ============================================================
// Behavioral Tests – Context cancellation
// ============================================================

func TestRateLimiterContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 10, Max: 100},
		WithAvailableTokens(0))
	require.NoError(t, err)

	cancel()

	// After context cancel, refill stops – wait and confirm no tokens appear
	time.Sleep(500 * time.Millisecond)
	// Allow should return false (no refill happened)
	require.False(t, rl.Allow())
}

// ============================================================
// Behavioral Tests – Rapid creation and destruction
// ============================================================

func TestRateLimiterRapidCreateDestroy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create and destroy many limiters rapidly to check for leaks/panics
	for i := 0; i < 100; i++ {
		rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 10, Max: 100})
		require.NoError(t, err)
		rl.Allow()
		rl.Close()
	}
}

// ============================================================
// Behavioral Tests – Different NPerSec tiers
// ============================================================

func TestRateLimiterAllTiers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test each tier: <=10, <=10000, >10000
	tests := []struct {
		name    string
		nPerSec int
		max     int
	}{
		{"tier1_low_1", 1, 10},
		{"tier1_low_10", 10, 100},
		{"tier2_medium_100", 100, 1000},
		{"tier2_medium_10000", 10000, 100000},
		{"tier3_high_20000", 20000, 200000},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rl, err := NewRateLimiter(ctx,
				RateLimiterArgs{NPerSec: tt.nPerSec, Max: tt.max},
				WithAvailableTokens(0))
			require.NoError(t, err)
			defer rl.Close()

			// Wait 1 second, check tokens are roughly NPerSec
			time.Sleep(1100 * time.Millisecond)
			tokens := rl.Len()
			tolerance := float64(tt.nPerSec) * 0.15
			if tolerance < 2 {
				tolerance = 2
			}
			require.InDelta(t, tt.nPerSec, tokens, tolerance,
				"tier %s: expected ~%d tokens after 1s, got %d",
				tt.name, tt.nPerSec, tokens)
		})
	}
}

// ============================================================
// Behavioral Tests – NPerSec equals Max boundary
// ============================================================

func TestRateLimiterNPerSecEqualsMax(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 5, Max: 5})
	require.NoError(t, err)
	defer rl.Close()

	// Should start with NPerSec tokens
	require.Equal(t, 5, rl.Len())

	// Consume all
	for i := 0; i < 5; i++ {
		require.True(t, rl.Allow())
	}
	require.False(t, rl.Allow())

	// After 1s, should refill to Max (which is same as NPerSec)
	time.Sleep(1100 * time.Millisecond)
	require.Equal(t, 5, rl.Len())
}

// ============================================================
// Behavioral Tests – Partial refill timing (tier-aware)
// ============================================================

// TestRateLimiterLowRateRefillGranularity verifies that low-rate limiters
// (NPerSec<=10) refill smoothly at sub-second intervals, not in 1s batches.
func TestRateLimiterLowRateRefillGranularity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 10, Max: 100},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	// After 500ms at 10/s with 100ms interval, ~5 ticks * 1 token = ~5
	time.Sleep(550 * time.Millisecond)
	tokens := rl.Len()
	require.InDelta(t, 5, tokens, 2,
		"expected ~5 tokens after 500ms at 10/s, got %d", tokens)
}

// TestRateLimiterTier2RefillGranularity checks that tier2 (10 < NPerSec <= 10000)
// refills every 100ms, providing smoother token delivery.
func TestRateLimiterTier2RefillGranularity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: 100, Max: 1000},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	// After 500ms at 100/s with 100ms interval, ~5 ticks * 10 tokens = ~50
	time.Sleep(550 * time.Millisecond)
	tokens := rl.Len()
	require.InDelta(t, 50, tokens, 15,
		"expected ~50 tokens after 500ms at 100/s tier2, got %d", tokens)
}

// ============================================================
// Behavioral Tests – Medium rate with continuous consumption
// ============================================================

func TestRateLimiterMediumRateContinuousConsumption(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const nPerSec = 100
	rl, err := NewRateLimiter(ctx,
		RateLimiterArgs{NPerSec: nPerSec, Max: nPerSec * 10},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	// Continuously consume for 2 seconds at a rate slightly below nPerSec
	var allowed int64
	done := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-done:
			break loop
		case <-ticker.C:
			if rl.Allow() {
				allowed++
			}
		}
	}

	// Should get roughly 200 tokens over 2 seconds
	require.InDelta(t, 200, allowed, 30,
		"expected ~200 allowed over 2s at 100/s, got %d", allowed)
}
