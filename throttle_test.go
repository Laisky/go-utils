package utils

import (
	"context"
	"fmt"
	"testing"
	"time"
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
