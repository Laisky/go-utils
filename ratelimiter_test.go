package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// ============================================================
// Unit Tests – constructor & argument validation
// ============================================================

// TestRateLimiter verifies NewRateLimiter argument validation (zero or negative NPerSec, Max below NPerSec, and
// a nil context are rejected), that limiters can be stopped via Close or context cancellation, and that Allow and
// AllowN admit the initial 10 tokens, reject further requests, and admit requests again after about one second
// of refill.
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

// TestRateLimiterAllowNEdgeCases verifies that AllowN always succeeds for zero or negative n, always fails for n
// greater than Max, can consume exactly Max tokens, and leaves the balance unchanged when it rejects a request.
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

// TestWithAvailableTokensOption verifies that WithAvailableTokens sets the initial token count (including zero,
// which makes Allow fail immediately) and that a negative count or a count above Max makes NewRateLimiter fail.
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

// TestRateLimiterStateLifecycle verifies that ExportState reports the tokens remaining after consumption, that
// RestoreState overwrites the token count when the args match, and that RestoreState rejects a state whose args
// differ from the limiter's.
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

// TestRateLimiterRestoreStateBounds verifies that RestoreState rejects a negative token count and a count above
// Max even when the state's args match the limiter's.
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

// TestRateLimiterClone verifies that Clone copies the args and current token count into an independent limiter,
// so consuming a token on the clone does not change the original's balance.
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

// TestNewRateLimiterWithStateOption verifies that the WithRateLimiterState option seeds a new limiter with the
// saved token count, that WithAvailableTokens above Max is rejected, and that a state whose args differ from the
// limiter's args makes NewRateLimiter fail.
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
// Benchmarks
// ============================================================

// BenchmarkRateLimiter measures the per-call cost of Allow under b.RunParallel for this package's RateLimiter and
// for golang.org/x/time/rate.Limiter, both configured for 10 tokens per second with a burst of 100. The recorded
// output below is from an earlier run.
//
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

// ExampleRateLimiter demonstrates creating a RateLimiter that refills 10 tokens per second up to 100 and using
// Allow to drop messages read from a channel whenever no token is available.
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
