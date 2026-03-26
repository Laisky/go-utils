package utils

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// ============================================================
// Unit Tests – constructor & argument validation
// ============================================================

func TestRateLimiter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("wrong args", func(t *testing.T) {
		t.Parallel()
		_, err := NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: 0,
			Max:     100,
		})
		require.Error(t, err)

		_, err = NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: 10,
			Max:     9,
		})
		require.Error(t, err)
	})

	t.Run("nil context", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck
		_, err := NewRateLimiter(nil, RateLimiterArgs{
			NPerSec: 10,
			Max:     100,
		})
		require.Error(t, err)
	})

	t.Run("negative npersec", func(t *testing.T) {
		t.Parallel()
		_, err := NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: -1,
			Max:     100,
		})
		require.Error(t, err)
	})

	t.Run("stop", func(t *testing.T) {
		t.Parallel()
		RateLimiter2, err := NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(t, err)
		RateLimiter2.Close()

		ctx2, cancel := context.WithCancel(ctx)
		_, err = NewRateLimiter(ctx2, RateLimiterArgs{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(t, err)
		cancel()
	})

	t.Run("allow", func(t *testing.T) {
		t.Parallel()

		ratelimiter, err := NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(t, err)
		defer ratelimiter.Close()

		for i := 0; i < 20; i++ {
			allowed := ratelimiter.Allow()
			if i < 10 {
				require.True(t, allowed, i)
			} else if i >= 10 {
				require.False(t, allowed, i)
			}
		}

		time.Sleep(1050 * time.Millisecond)
		require.GreaterOrEqual(t, ratelimiter.Len(), 10)
		for i := 0; i < 5; i++ {
			require.True(t, ratelimiter.Allow(), i)
		}
		require.GreaterOrEqual(t, ratelimiter.Len(), 5)
		require.False(t, ratelimiter.AllowN(20))
		require.GreaterOrEqual(t, ratelimiter.Len(), 5)
		require.True(t, ratelimiter.AllowN(5))

		for i := 0; i < 100; i++ {
			require.False(t, ratelimiter.Allow(), i)
		}
	})
}

// ============================================================
// Unit Tests – AllowN edge cases
// ============================================================

func TestRateLimiterAllowNEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("allow zero always succeeds", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 1, Max: 1})
		require.NoError(t, err)
		defer rl.Close()

		// Drain all tokens
		rl.Allow()

		// AllowN(0) should always return true even when empty
		require.True(t, rl.AllowN(0))
		require.True(t, rl.AllowN(-1))
	})

	t.Run("allow n exceeding max always fails", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 5, Max: 10})
		require.NoError(t, err)
		defer rl.Close()

		// Even with full tokens, requesting more than Max should fail
		require.False(t, rl.AllowN(11))
		// Tokens should not be consumed by a failed AllowN
		require.Equal(t, 5, rl.Len())
	})

	t.Run("allow exactly max tokens", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 10, Max: 10},
			WithAvailableTokens(10))
		require.NoError(t, err)
		defer rl.Close()

		require.True(t, rl.AllowN(10))
		require.Equal(t, 0, rl.Len())
	})

	t.Run("tokens not consumed on insufficient balance", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 5, Max: 10},
			WithAvailableTokens(3))
		require.NoError(t, err)
		defer rl.Close()

		// Try to consume more than available but within Max
		require.False(t, rl.AllowN(4))
		// Tokens should remain unchanged
		require.Equal(t, 3, rl.Len())
	})
}

// ============================================================
// Unit Tests – WithAvailableTokens option
// ============================================================

func TestWithAvailableTokensOption(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("set initial tokens", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 5, Max: 20},
			WithAvailableTokens(15))
		require.NoError(t, err)
		defer rl.Close()

		require.Equal(t, 15, rl.Len())
	})

	t.Run("zero initial tokens", func(t *testing.T) {
		t.Parallel()
		rl, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 5, Max: 20},
			WithAvailableTokens(0))
		require.NoError(t, err)
		defer rl.Close()

		require.Equal(t, 0, rl.Len())
		require.False(t, rl.Allow())
	})

	t.Run("negative tokens rejected", func(t *testing.T) {
		t.Parallel()
		_, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 5, Max: 20},
			WithAvailableTokens(-1))
		require.Error(t, err)
	})

	t.Run("exceeding max rejected", func(t *testing.T) {
		t.Parallel()
		_, err := NewRateLimiter(ctx,
			RateLimiterArgs{NPerSec: 5, Max: 20},
			WithAvailableTokens(21))
		require.Error(t, err)
	})
}

// ============================================================
// Unit Tests – State lifecycle (export / restore / clone)
// ============================================================

func TestRateLimiterStateLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	limiter, err := NewRateLimiter(ctx, RateLimiterArgs{
		NPerSec: 5,
		Max:     10,
	})
	require.NoError(t, err)
	t.Cleanup(limiter.Close)

	require.True(t, limiter.AllowN(3))

	state := limiter.ExportState()
	require.Equal(t, 2, state.AvailableTokens)

	err = limiter.RestoreState(RateLimiterState{
		Args:            limiter.RateLimiterArgs,
		AvailableTokens: 7,
	})
	require.NoError(t, err)
	require.Equal(t, 7, limiter.Len())

	err = limiter.RestoreState(RateLimiterState{
		Args:            RateLimiterArgs{NPerSec: 6, Max: 10},
		AvailableTokens: 1,
	})
	require.Error(t, err)
}

func TestRateLimiterRestoreStateBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rl, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 5, Max: 10})
	require.NoError(t, err)
	defer rl.Close()

	// Negative tokens
	err = rl.RestoreState(RateLimiterState{
		Args:            rl.RateLimiterArgs,
		AvailableTokens: -1,
	})
	require.Error(t, err)

	// Exceeding max
	err = rl.RestoreState(RateLimiterState{
		Args:            rl.RateLimiterArgs,
		AvailableTokens: 11,
	})
	require.Error(t, err)
}

func TestRateLimiterClone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	limiter, err := NewRateLimiter(ctx, RateLimiterArgs{
		NPerSec: 4,
		Max:     8,
	})
	require.NoError(t, err)
	t.Cleanup(limiter.Close)

	require.True(t, limiter.AllowN(2))

	clone, err := limiter.Clone(ctx)
	require.NoError(t, err)
	t.Cleanup(clone.Close)

	require.Equal(t, limiter.RateLimiterArgs, clone.RateLimiterArgs)
	require.Equal(t, limiter.Len(), clone.Len())

	// Clone is independent – consuming on clone doesn't affect original
	require.True(t, clone.Allow())
	require.Equal(t, 2, limiter.Len()) // original unchanged
	require.Equal(t, 1, clone.Len())
}

func TestNewRateLimiterWithStateOption(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	state := RateLimiterState{
		Args: RateLimiterArgs{
			NPerSec: 3,
			Max:     6,
		},
		AvailableTokens: 1,
	}

	limiter, err := NewRateLimiter(ctx, state.Args, WithRateLimiterState(state))
	require.NoError(t, err)
	t.Cleanup(limiter.Close)

	require.Equal(t, 1, limiter.Len())

	_, err = NewRateLimiter(ctx, state.Args, WithAvailableTokens(7))
	require.Error(t, err)

	// Mismatched args
	_, err = NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 3, Max: 6},
		WithRateLimiterState(RateLimiterState{
			Args:            RateLimiterArgs{NPerSec: 4, Max: 6},
			AvailableTokens: 1,
		}))
	require.Error(t, err)
}

// ============================================================
// Unit Tests – MemoryRateLimiterStateManager
// ============================================================

func TestMemoryRateLimiterStateManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	shouldRefill, err := manager.Setup(ctx, args, 3)
	require.NoError(t, err)
	require.True(t, shouldRefill)

	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, tokens)

	added, err := manager.AddTokens(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 7, added)

	ok, err := manager.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = manager.TryConsume(ctx, 20)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, manager.SetAvailableTokens(ctx, 2))

	shouldRefill, err = manager.Setup(ctx, args, 6)
	require.NoError(t, err)
	require.False(t, shouldRefill)

	added, err = manager.AddTokens(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, 5, added)

	tokens, err = manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, tokens)
}

func TestMemoryStateManagerUninitialized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()

	// All operations on an uninitialized manager should fail
	_, err := manager.TryConsume(ctx, 1)
	require.Error(t, err)

	_, err = manager.AddTokens(ctx, 1)
	require.Error(t, err)

	_, err = manager.AvailableTokens(ctx)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 1)
	require.Error(t, err)
}

func TestMemoryStateManagerCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	manager := NewMemoryRateLimiterStateManager()

	_, err := manager.Setup(ctx, RateLimiterArgs{NPerSec: 1, Max: 1}, 1)
	require.Error(t, err)

	// Initialize with a good context first
	goodCtx := context.Background()
	_, err = manager.Setup(goodCtx, RateLimiterArgs{NPerSec: 1, Max: 1}, 1)
	require.NoError(t, err)

	_, err = manager.TryConsume(ctx, 1)
	require.Error(t, err)

	_, err = manager.AddTokens(ctx, 1)
	require.Error(t, err)

	_, err = manager.AvailableTokens(ctx)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 1)
	require.Error(t, err)
}

func TestMemoryStateManagerAddTokensEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 10)
	require.NoError(t, err)

	// Already at max, adding should return 0
	added, err := manager.AddTokens(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, 0, added)

	// Adding zero or negative returns 0
	added, err = manager.AddTokens(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, 0, added)

	added, err = manager.AddTokens(ctx, -1)
	require.NoError(t, err)
	require.Equal(t, 0, added)
}

func TestMemoryStateManagerTryConsumeEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	// Consuming 0 always succeeds
	ok, err := manager.TryConsume(ctx, 0)
	require.NoError(t, err)
	require.True(t, ok)

	// Consuming negative always succeeds
	ok, err = manager.TryConsume(ctx, -1)
	require.NoError(t, err)
	require.True(t, ok)

	// Tokens unchanged after zero/negative consume
	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 5, tokens)
}

func TestMemoryStateManagerSetupMismatchedArgs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	// Second setup with different args should fail
	_, err = manager.Setup(ctx, RateLimiterArgs{NPerSec: 3, Max: 10}, 3)
	require.Error(t, err)
}

func TestMemoryStateManagerSetAvailableTokensBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	err = manager.SetAvailableTokens(ctx, -1)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 11)
	require.Error(t, err)

	// Valid boundary values
	require.NoError(t, manager.SetAvailableTokens(ctx, 0))
	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, tokens)

	require.NoError(t, manager.SetAvailableTokens(ctx, 10))
	tokens, err = manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 10, tokens)
}

// ============================================================
// Unit Tests – Shared state manager
// ============================================================

func TestRateLimiterWithSharedStateManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 2, Max: 4}

	limiterA, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterA.Close)

	limiterB, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterB.Close)

	require.True(t, limiterA.Allow())
	require.True(t, limiterB.Allow())

	require.False(t, limiterA.Allow())
	require.False(t, limiterB.Allow())

	time.Sleep(1100 * time.Millisecond)

	require.True(t, limiterA.Allow())
	require.True(t, limiterB.Allow())
	require.False(t, limiterA.Allow())
}

func TestWithRateLimiterStateManagerNilRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 1, Max: 1},
		WithRateLimiterStateManager(nil))
	require.Error(t, err)
}

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

// ============================================================
// Benchmarks
// ============================================================

/*
goos: linux
goarch: amd64
pkg: github.com/Laisky/go-utils
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
BenchmarkRateLimiter/RateLimiter-8            684580170                1.553 ns/op           0 B/op          0 allocs/op
BenchmarkRateLimiter/rate.Limiter-8         4633182               309.2 ns/op             0 B/op          0 allocs/op
*/
func BenchmarkRateLimiter(b *testing.B) {
	ctx := context.Background()
	b.Run("RateLimiter", func(b *testing.B) {
		RateLimiter, err := NewRateLimiter(ctx, RateLimiterArgs{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(b, err)
		defer RateLimiter.Close()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				RateLimiter.Allow()
			}
		})
	})

	b.Run("golang.org/x/time/rate", func(b *testing.B) {
		limiter := rate.NewLimiter(rate.Limit(10), 100)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Allow()
			}
		})
	})
}

func ExampleRateLimiter() {
	ctx := context.Background()
	RateLimiter, err := NewRateLimiter(ctx, RateLimiterArgs{
		NPerSec: 10,
		Max:     100,
	})
	if err != nil {
		panic("new RateLimiter")
	}
	defer RateLimiter.Close()

	inChan := make(chan int)

	for msg := range inChan {
		if !RateLimiter.Allow() {
			continue
		}

		// do something with msg
		fmt.Println(msg)
	}
}
