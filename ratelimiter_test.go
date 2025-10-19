package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

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
}

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
