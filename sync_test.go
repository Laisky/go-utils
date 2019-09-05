package utils_test

import (
	"testing"
	"time"

	"github.com/Laisky/go-utils"
)

func TestMutex(t *testing.T) {
	l := utils.NewMutex()
	if !l.TryLock() {
		t.Fatal("should acquire lock")
	}
	if !l.IsLocked() {
		t.Fatal("should locked")
	}
	if l.TryLock() {
		t.Fatal("should not acquire lock")
	}
	if !l.TryRelease() {
		t.Fatal("should release lock")
	}
	if l.IsLocked() {
		t.Fatal("should not locked")
	}
	if l.TryRelease() {
		t.Fatal("should not release lock")
	}
	l.SpinLock(1*time.Second, 3*time.Second)
	if !l.IsLocked() {
		t.Fatal("should locked")
	}
	start := time.Now()
	l.SpinLock(1*time.Second, 3*time.Second)
	if time.Now().Sub(start) < 3*time.Second || time.Now().Sub(start) > 4100*time.Millisecond {
		t.Fatalf("duration: %v", time.Now().Sub(start).Seconds())
	}

	l.ForceRelease()
	if l.IsLocked() {
		t.Fatal("should not locked")
	}
}

func ExampleMutex() {
	l := utils.NewMutex()
	if !l.TryLock() {
		utils.Logger.Info("can not acquire lock")
		return
	}
	defer l.ForceRelease()

}

func BenchmarkMutex(b *testing.B) {
	l := utils.NewMutex()
	// step := 1 * time.Millisecond
	// timeoout := 1 * time.Second
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				l.TryLock()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				l.TryRelease()
			}
		})
		// b.RunParallel(func(pb *testing.PB) {
		// 	for pb.Next() {
		// 		l.SpinLock(step, timeoout)
		// 	}
		// })
	})
}
