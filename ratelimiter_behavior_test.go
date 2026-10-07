package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The behavioral tests in this file run the real background refill. A loaded host can delay or starve that
// goroutine, and this test goroutine, for an unbounded time, so instead of comparing counts at fixed sleep
// deadlines every test checks two load-independent properties: the bucket never hands out tokens faster than
// NPerSec over the measured elapsed time (requireWithinRefillRate), and it eventually delivers the expected tokens
// (awaitTokens, consumeUntil). Exact refill arithmetic is covered with a driven clock in
// ratelimiter_refill_regression_test.go and by the "driven" subtests below.

// ============================================================
// Behavioral Tests – Token refill accuracy
// ============================================================

// TestRateLimiterRefillAccuracy verifies that limiters starting with zero tokens refill toward the configured rate
// and never faster: they reach 5 tokens at 3/s, 85 at 100/s and 45000 at 50000/s, and at that point hold no more
// than NPerSec times the measured time since their creation.
func TestRateLimiterRefillAccuracy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args RateLimiterArgs
		want int
	}{
		{"low rate: refill 3 per sec", RateLimiterArgs{NPerSec: 3, Max: 30}, 5},
		{"medium rate: refill 100 per sec", RateLimiterArgs{NPerSec: 100, Max: 1000}, 85},
		{"high rate: refill 50000 per sec", RateLimiterArgs{NPerSec: 50000, Max: 200000}, 45000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			rl, err := NewRateLimiter(context.Background(), tt.args, WithAvailableTokens(0))
			require.NoError(t, err)
			defer rl.Close()

			awaitTokens(t, rl, tt.want)
			requireWithinRefillRate(t, rl, 0, 0, start)
		})
	}
}

// TestRateLimiterMaxCap verifies tokens never exceed Max: a 10/s limiter capped at 15 fills up to exactly 15 and
// stays there while further refills keep arriving.
func TestRateLimiterMaxCap(t *testing.T) {
	t.Parallel()

	rl, err := NewRateLimiter(context.Background(),
		RateLimiterArgs{NPerSec: 10, Max: 15},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	awaitTokens(t, rl, 15)
	require.Never(t, func() bool { return rl.Len() > 15 }, 500*time.Millisecond, 10*time.Millisecond,
		"tokens should never exceed Max=15")
	require.Equal(t, 15, rl.Len())
}

// ============================================================
// Behavioral Tests – Sustained throughput over time
// ============================================================

// TestRateLimiterSustainedThroughput verifies that a limiter at 50 tokens per second that starts empty serves a
// consumer polling about every millisecond at no more than that rate: all 150 requests are eventually admitted,
// and admitting them takes at least the time 50/s allows.
func TestRateLimiterSustainedThroughput(t *testing.T) {
	t.Parallel()

	const nPerSec = 50
	start := time.Now()
	rl, err := NewRateLimiter(context.Background(),
		RateLimiterArgs{NPerSec: nPerSec, Max: nPerSec * 10},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	elapsed := consumeUntil(t, rl, 3*nPerSec, start)
	t.Logf("admitted %d requests in %s at %d/s", 3*nPerSec, elapsed, nPerSec)
	requireWithinRefillRate(t, rl, 0, 3*nPerSec, start)
}

// ============================================================
// Behavioral Tests – Burst and recovery
// ============================================================

// TestRateLimiterBurstAndRecovery verifies that a full limiter (10/s, Max 20) admits a back-to-back burst of its
// 20 tokens (plus at most what was refilled meanwhile) and then rejects, recovers tokens through the background
// refill, and lets the recovered tokens be consumed again, never exceeding the refill rate over measured time.
func TestRateLimiterBurstAndRecovery(t *testing.T) {
	t.Parallel()

	start := time.Now()
	rl, err := NewRateLimiter(context.Background(),
		RateLimiterArgs{NPerSec: 10, Max: 20},
		WithAvailableTokens(20))
	require.NoError(t, err)
	defer rl.Close()

	burst := 0
	for rl.Allow() {
		burst++
	}
	require.GreaterOrEqual(t, burst, 20, "the full bucket must be available as one burst")
	requireWithinRefillRate(t, rl, 20, burst, start)

	awaitTokens(t, rl, 8)
	recovered := 0
	for rl.Allow() {
		recovered++
	}
	require.GreaterOrEqual(t, recovered, 8, "recovered tokens must be consumable again")
	requireWithinRefillRate(t, rl, 20, burst+recovered, start)
}

// ============================================================
// Behavioral Tests – Concurrent access correctness
// ============================================================

// TestRateLimiterConcurrentAccess verifies that 50 goroutines each calling Allow 100 times against a limiter
// pre-filled with 1000 tokens get every initial token, and never more than the initial tokens plus what the
// refill could add over the measured time, so concurrent consumption cannot over-spend the bucket.
func TestRateLimiterConcurrentAccess(t *testing.T) {
	t.Parallel()

	const maxTokens = 1000
	start := time.Now()
	rl, err := NewRateLimiter(context.Background(),
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

	// 5000 attempts against 1000 tokens: every attempt succeeds while tokens remain, so all initial tokens go.
	allowed := int(totalAllowed.Load())
	t.Logf("concurrent test: %d/%d requests allowed", allowed, numGoroutines*100)
	require.GreaterOrEqual(t, allowed, maxTokens)
	requireWithinRefillRate(t, rl, maxTokens, allowed, start)
}

// TestRateLimiterConcurrentAllowN verifies that goroutines concurrently calling AllowN with mixed sizes
// (1, 2, 3, 5, 7, and 10) against a limiter pre-filled with 500 tokens consume some tokens and never more than
// the initial tokens plus what the refill could add over the measured time.
func TestRateLimiterConcurrentAllowN(t *testing.T) {
	t.Parallel()

	start := time.Now()
	rl, err := NewRateLimiter(context.Background(),
		RateLimiterArgs{NPerSec: 50, Max: 500},
		WithAvailableTokens(500))
	require.NoError(t, err)
	defer rl.Close()

	// Concurrently consume with varying AllowN sizes
	var totalConsumed atomic.Int64
	var wg sync.WaitGroup

	for _, n := range []int{1, 2, 5, 10, 3, 7} {
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

	consumed := int(totalConsumed.Load())
	t.Logf("concurrent AllowN: consumed %d tokens", consumed)
	require.Positive(t, consumed)
	requireWithinRefillRate(t, rl, 500, consumed, start)
}

// ============================================================
// Behavioral Tests – Token count never goes negative
// ============================================================

// TestRateLimiterTokensNeverNegative verifies that repeatedly calling AllowN(3) on a limiter that starts with a
// single token never drives Len below zero.
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

// TestRateLimiterContextCancellation verifies that canceling the context passed to NewRateLimiter stops the
// limiter: it rejects Allow afterwards, and the shared token pool, read directly from its state manager, gains no
// tokens after the cancellation, so 500ms later it holds no more than the refill could add before it.
func TestRateLimiterContextCancellation(t *testing.T) {
	t.Parallel()
	args := RateLimiterArgs{NPerSec: 10, Max: 100}
	manager := NewMemoryRateLimiterStateManager()

	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	rl, err := NewRateLimiter(ctx, args, WithAvailableTokens(0), WithRateLimiterStateManager(manager))
	require.NoError(t, err)

	cancel()
	canceled := time.Now()
	require.False(t, rl.Allow(), "a canceled limiter must reject requests")

	// A refill still running after the cancellation would add about 5 tokens over this window.
	time.Sleep(500 * time.Millisecond)
	tokens, err := manager.AvailableTokens(context.Background())
	require.NoError(t, err)
	require.LessOrEqual(t, float64(tokens), float64(args.NPerSec)*canceled.Sub(start).Seconds()+1,
		"tokens were added after the context was canceled")
}

// ============================================================
// Behavioral Tests – Rapid creation and destruction
// ============================================================

// TestRateLimiterRapidCreateDestroy verifies that creating, using, and closing 100 limiters in quick succession
// succeeds every time without errors or panics.
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

// TestRateLimiterAllTiers verifies that limiters starting empty accumulate roughly NPerSec tokens (within 15%,
// and at least 1) for rates from 1/s to 20000/s, covering both the 100ms refill interval and the 10ms interval
// used above 10000/s, and never hold more tokens than NPerSec times the measured time since their creation.
func TestRateLimiterAllTiers(t *testing.T) {
	t.Parallel()

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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			rl, err := NewRateLimiter(context.Background(),
				RateLimiterArgs{NPerSec: tt.nPerSec, Max: tt.max},
				WithAvailableTokens(0))
			require.NoError(t, err)
			defer rl.Close()

			tolerance := max(int(float64(tt.nPerSec)*0.15), 2)
			awaitTokens(t, rl, max(tt.nPerSec-tolerance, 1))
			requireWithinRefillRate(t, rl, 0, 0, start)
		})
	}
}

// ============================================================
// Behavioral Tests – NPerSec equals Max boundary
// ============================================================

// TestRateLimiterNPerSecEqualsMax verifies the boundary where NPerSec equals Max (5): a new limiter starts with
// 5 tokens, rejects the sixth Allow after five succeed, refills exactly to 5 after one second, and never exceeds
// 5 however long it refills. The exact counts use a driven refill loop; the real background refill is checked
// to restore the full bucket without a deadline tied to host load.
func TestRateLimiterNPerSecEqualsMax(t *testing.T) {
	t.Parallel()
	args := RateLimiterArgs{NPerSec: 5, Max: 5}

	t.Run("driven refill", func(t *testing.T) {
		t.Parallel()
		rl, refill := newManuallyRefilledLimiter(t, args)

		for i := 0; i < 5; i++ {
			require.True(t, rl.Allow(), i)
		}
		require.False(t, rl.Allow())

		refill.advance(time.Second)
		require.Equal(t, 5, rl.Len())

		refill.advance(time.Hour)
		require.Equal(t, 5, rl.Len(), "refill must stop at Max")
	})

	t.Run("real refill", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(context.Background(), args)
		require.NoError(t, err)
		defer rl.Close()

		// The default initial balance is NPerSec, which is also Max, so no refill can change it.
		require.Equal(t, 5, rl.Len())
		for i := 0; i < 5; i++ {
			require.True(t, rl.Allow(), i)
		}

		require.Eventually(t, func() bool { return rl.Len() == 5 },
			30*time.Second, 10*time.Millisecond, "the background refill must restore the full bucket")
		require.Equal(t, 5, rl.Len(), "refill must stop at Max")
	})
}

// ============================================================
// Behavioral Tests – Partial refill timing (tier-aware)
// ============================================================

// TestRateLimiterLowRateRefillGranularity verifies that low-rate limiters (NPerSec<=10) refill in proportion to
// elapsed time at sub-second steps, not in 1s batches: with a driven clock, 100ms at 10/s yields exactly one token
// and 500ms in total yields five; the real background refill delivers tokens without exceeding the rate.
func TestRateLimiterLowRateRefillGranularity(t *testing.T) {
	t.Parallel()
	args := RateLimiterArgs{NPerSec: 10, Max: 100}

	t.Run("driven", func(t *testing.T) {
		t.Parallel()
		rl, refill := newManuallyRefilledLimiter(t, args, WithAvailableTokens(0))
		refill.advance(100 * time.Millisecond)
		require.Equal(t, 1, rl.Len())
		refill.advance(400 * time.Millisecond)
		require.Equal(t, 5, rl.Len())
	})

	t.Run("real", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		rl, err := NewRateLimiter(context.Background(), args, WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		awaitTokens(t, rl, 3)
		requireWithinRefillRate(t, rl, 0, 0, start)
	})
}

// TestRateLimiterTier2RefillGranularity checks that tier2 (10 < NPerSec <= 10000) refills in proportion to elapsed
// time at 100ms steps: with a driven clock, 100ms at 100/s yields exactly 10 tokens and 500ms in total yields 50;
// the real background refill delivers tokens without exceeding the rate.
func TestRateLimiterTier2RefillGranularity(t *testing.T) {
	t.Parallel()
	args := RateLimiterArgs{NPerSec: 100, Max: 1000}

	t.Run("driven", func(t *testing.T) {
		t.Parallel()
		rl, refill := newManuallyRefilledLimiter(t, args, WithAvailableTokens(0))
		refill.advance(100 * time.Millisecond)
		require.Equal(t, 10, rl.Len())
		refill.advance(400 * time.Millisecond)
		require.Equal(t, 50, rl.Len())
	})

	t.Run("real", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		rl, err := NewRateLimiter(context.Background(), args, WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		awaitTokens(t, rl, 35)
		requireWithinRefillRate(t, rl, 0, 0, start)
	})
}

// ============================================================
// Behavioral Tests – Medium rate with continuous consumption
// ============================================================

// TestRateLimiterMediumRateContinuousConsumption verifies that a limiter at 100 tokens per second that starts
// empty serves a consumer polling about every millisecond at no more than that rate: all 200 requests are
// eventually admitted, and admitting them takes at least the time 100/s allows.
func TestRateLimiterMediumRateContinuousConsumption(t *testing.T) {
	t.Parallel()

	const nPerSec = 100
	start := time.Now()
	rl, err := NewRateLimiter(context.Background(),
		RateLimiterArgs{NPerSec: nPerSec, Max: nPerSec * 10},
		WithAvailableTokens(0))
	require.NoError(t, err)
	defer rl.Close()

	elapsed := consumeUntil(t, rl, 2*nPerSec, start)
	t.Logf("admitted %d requests in %s at %d/s", 2*nPerSec, elapsed, nPerSec)
	requireWithinRefillRate(t, rl, 0, 2*nPerSec, start)
}
