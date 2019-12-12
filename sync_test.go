package utils_test

import (
	"context"
	"github.com/Laisky/zap"
	"strings"
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
	if time.Since(start) < 3*time.Second || time.Since(start) > 4100*time.Millisecond {
		t.Fatalf("duration: %v", time.Since(start).Seconds())
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

func TestLaiskyRemoteLock(t *testing.T) {
	// utils.Logger.ChangeLevel("debug")
	cli, err := utils.NewLaiskyRemoteLock(
		"https://blog.laisky.com/graphql/query/",
		"eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NzYxMzUyODAsInVpZCI6ImxhaXNreSJ9.iPz0QUTw0ASld2jR8JJ1yaMrnOCj6SShu1U2NdTNFtKCjwMV64N-b0bhu8MAkqdUmrqgRkD1zstE39GutYF-YA",
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ctx := context.Background()
	if ok, err := cli.AcquireLock(ctx, "laisky.test", 10*time.Second, true); err != nil {
		if !strings.Contains(err.Error(), "Token is expired") {
			t.Fatalf("%+v", err)
		}
	} else if !ok {
		t.Logf("not ok")
	}

	time.Sleep(3 * time.Second)
	// t.Error("done")
}

func ExampleLaiskyRemoteLock() {
	cli, err := utils.NewLaiskyRemoteLock(
		"https://blog.laisky.com/graphql/query/",
		"eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NzYxMzUyODAsInVpZCI6ImxhaXNreSJ9.iPz0QUTw0ASld2jR8JJ1yaMrnOCj6SShu1U2NdTNFtKCjwMV64N-b0bhu8MAkqdUmrqgRkD1zstE39GutYF-YA",
	)
	if err != nil {
		utils.Logger.Error("create laisky lock", zap.Error(err))
	}

	var (
		ok          bool
		lockName    = "laisky.test"
		ctx, cancel = context.WithCancel(context.Background())
	)
	defer cancel()
	if ok, err = cli.AcquireLock(ctx, lockName, 10*time.Second, true); err != nil {
		utils.Logger.Error("acquire lock", zap.String("lock_name", lockName))
	}

	if ok {
		utils.Logger.Info("success acquired lock")
	} else {
		utils.Logger.Info("do not acquired lock")
		return
	}

	time.Sleep(3 * time.Second) // will auto renewal lock in background
}
