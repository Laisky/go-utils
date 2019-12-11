package utils_test

import (
	"context"
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

func TestLaiskyRemoteLockOptFunc(t *testing.T) {
	cli, err := utils.NewLaiskyRemoteLock(
		"https://blog.laisky.com/graphql/query/",
		"eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NzYwNzU5MzgsInVpZCI6InRlc3QifQ.Qm38mdHPViMxkYml7zQ_wFkqDhoHnv29JjVblvxfITEA9EftXPZQdtETuspK4WwjPWRR6QPHQ13hNFM0PwSulw",
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ctx := context.Background()
	if ok, err := cli.AcquireLock(ctx, "test.foo", 30*time.Second, true); err != nil {
		if !strings.Contains(err.Error(), "do not have permission") {
			t.Fatalf("%+v", err)
		}
	} else if !ok {
		t.Logf("not ok")
	}

	// t.Error("done")
}
