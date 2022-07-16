package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestThrottle2(t *testing.T) {
	ctx := context.Background()
	throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
		NPerSec: 10,
		Max:     100,
	})
	require.NoError(t, err)
	defer throttle.Close()

	// case: wrong args
	{
		_, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
			NPerSec: 0,
			Max:     100,
		})
		require.Error(t, err)

		_, err = NewThrottleWithCtx(ctx, &ThrottleCfg{
			NPerSec: 10,
			Max:     9,
		})
		require.Error(t, err)
	}

	// case: stop
	{
		throttle2, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(t, err)
		throttle2.Close()

		ctx2, cancel := context.WithCancel(ctx)
		_, err = NewThrottleWithCtx(ctx2, &ThrottleCfg{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(t, err)
		cancel()
	}

	for i := 0; i < 20; i++ {
		allowed := throttle.Allow()
		if i < 10 {
			require.True(t, allowed, i)
		} else if i >= 10 {
			require.False(t, allowed, i)
		}
	}

	time.Sleep(2050 * time.Millisecond)
	for i := 0; i < 20; i++ {
		require.True(t, throttle.Allow(), i)
	}

	for i := 0; i < 100; i++ {
		require.False(t, throttle.Allow(), i)
	}
}

/*
goos: linux
goarch: amd64
pkg: github.com/Laisky/go-utils
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
BenchmarkThrottle/throttle-8            684580170                1.553 ns/op           0 B/op          0 allocs/op
BenchmarkThrottle/rate.Limiter-8         4633182               309.2 ns/op             0 B/op          0 allocs/op
*/
func BenchmarkThrottle(b *testing.B) {
	ctx := context.Background()
	b.Run("throttle", func(b *testing.B) {
		throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
			NPerSec: 10,
			Max:     100,
		})
		require.NoError(b, err)
		defer throttle.Close()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				throttle.Allow()
			}
		})
	})

	b.Run("rate.Limiter", func(b *testing.B) {
		limiter := rate.NewLimiter(rate.Limit(10), 100)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Allow()
			}
		})
	})
}

func ExampleThrottle() {
	ctx := context.Background()
	throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
		NPerSec: 10,
		Max:     100,
	})
	if err != nil {
		panic("new throttle")
	}
	defer throttle.Close()

	inChan := make(chan int)

	for msg := range inChan {
		if !throttle.Allow() {
			continue
		}

		// do something with msg
		fmt.Println(msg)
	}
}
