package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestThrottle2(t *testing.T) {
	ctx := context.Background()
	throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
		NPerSec: 10,
		Max:     100,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer throttle.Close()

	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 20; i++ {
		if !throttle.Allow() {
			if i < 10 {
				t.Fatalf("should be allowed: %v", i)
			} else {
				break
			}
		}
	}

	time.Sleep(2050 * time.Millisecond)
	for i := 0; i < 20; i++ {
		if !throttle.Allow() {
			t.Fatalf("should be allowed: %v", i)
		}
	}

	for i := 0; i < 100; i++ {
		if throttle.Allow() {
			t.Errorf("should not be allowed: %v", i)
		}
	}
}

// BenchmarkThrottle/throttle-8         	13897974	        85.3 ns/op	       0 B/op	       0 allocs/op
// BenchmarkThrottle/rate.Limiter-8     	  148858	      7344 ns/op	       0 B/op	       0 allocs/op
func BenchmarkThrottle(b *testing.B) {
	ctx := context.Background()
	b.Run("throttle", func(b *testing.B) {
		throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
			NPerSec: 10,
			Max:     100,
		})
		if err != nil {
			b.Fatalf("%+v", err)
		}
		defer throttle.Close()

		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					throttle.Allow()
				}
			})
		}
	})

	b.Run("rate.Limiter", func(b *testing.B) {
		limiter := rate.NewLimiter(rate.Limit(10), 100)
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					limiter.Allow()
				}
			})
		}
	})
}

func ExampleThrottle() {
	ctx := context.Background()
	throttle, err := NewThrottleWithCtx(ctx, &ThrottleCfg{
		NPerSec: 10,
		Max:     100,
	})
	if err != nil {
		Logger.Panic("new throttle")
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
