package utils_test

import (
	"testing"
	"time"

	"github.com/Laisky/go-utils"
)

func TestThrottle(t *testing.T) {
	throttle := utils.NewThrottle(&utils.ThrottleCfg{
		NPerSec: 10,
		Max:     100,
	})
	throttle.Run()

	time.Sleep(1050 * time.Millisecond)
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
