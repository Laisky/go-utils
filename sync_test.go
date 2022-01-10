package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMutex(t *testing.T) {
	l := NewMutex()
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
	l := NewMutex()
	if !l.TryLock() {
		Logger.Info("can not acquire lock")
		return
	}
	defer l.ForceRelease()

}

func BenchmarkMutex(b *testing.B) {
	l := NewMutex()
	// step := 1 * time.Millisecond
	// timeoout := 1 * time.Second
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				l.TryLock()
				l.TryRelease()
			}
		})
		// b.RunParallel(func(pb *testing.PB) {
		// 	for pb.Next() {
		// 		l.TryRelease()
		// 	}
		// })
		// b.RunParallel(func(pb *testing.PB) {
		// 	for pb.Next() {
		// 		l.SpinLock(step, timeoout)
		// 	}
		// })
	})
}

// func TestLaiskyRemoteLock(t *testing.T) {
// 	// Logger.ChangeLevel("debug")
// 	cli, err := NewLaiskyRemoteLock(
// 		"https://blog.laisky.com/graphql/query/",
// 		"eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NzYxMzQxMDAsInVpZCI6ImxhaXNreSJ9.r9YTtrU7RO0qMDKA8rAYXI0bzya9JYGam1l-dFxnHOAYD9qXhYXfubUfi_yo5LgDBBOON9XSkl2kIGrqqQWlyA",
// 	)
// 	if err != nil {
// 		t.Fatalf("%+v", err)
// 	}

// 	ctx := context.Background()
// 	if ok, err := cli.AcquireLock(
// 		ctx,
// 		"laisky.test",
// 		WithAcquireLockDuration(10*time.Second),
// 		WithAcquireLockIsRenewal(true),
// 	); err != nil {
// 		if !strings.Contains(err.Error(), "Token is expired") {
// 			t.Fatalf("%+v", err)
// 		}
// 	} else if !ok {
// 		t.Logf("not ok")
// 	}

// 	time.Sleep(3 * time.Second)
// 	// t.Error("done")
// }

// func ExampleLaiskyRemoteLock() {
// 	cli, err := NewLaiskyRemoteLock(
// 		"https://blog.laisky.com/graphql/query/",
// 		"eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NzYxMzQxMDAsInVpZCI6ImxhaXNreSJ9.r9YTtrU7RO0qMDKA8rAYXI0bzya9JYGam1l-dFxnHOAYD9qXhYXfubUfi_yo5LgDBBOON9XSkl2kIGrqqQWlyA",
// 	)
// 	if err != nil {
// 		Logger.Error("create laisky lock", zap.Error(err))
// 	}

// 	var (
// 		ok          bool
// 		lockName    = "laisky.test"
// 		ctx, cancel = context.WithCancel(context.Background())
// 	)
// 	defer cancel()
// 	if ok, err = cli.AcquireLock(
// 		ctx,
// 		lockName,
// 		WithAcquireLockDuration(10*time.Second),
// 		WithAcquireLockIsRenewal(true),
// 	); err != nil {
// 		Logger.Error("acquire lock", zap.String("lock_name", lockName))
// 	}

// 	if ok {
// 		Logger.Info("success acquired lock")
// 	} else {
// 		Logger.Info("do not acquired lock")
// 		return
// 	}

// 	time.Sleep(3 * time.Second) // will auto renewal lock in background
// }

func TestNewExpiredRLock(t *testing.T) {
	lm, err := NewExpiredRLock(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	k := "yo"
	l := lm.GetLock(k)

	l.RLock()
	l.RLock()
	go func() {
		l.Lock()
	}()

	time.Sleep(time.Millisecond)
	l.RUnlock()
	l.RUnlock()
}

func ExampleRunWithTimeout() {
	slow := func() { time.Sleep(10 * time.Second) }
	startAt := time.Now()
	RunWithTimeout(5*time.Millisecond, slow)

	fmt.Println(time.Since(startAt) < 10*time.Second)
	// Output:
	// true
}

func TestRunWithTimeout(t *testing.T) {
	slow := func() { time.Sleep(10 * time.Second) }
	startAt := time.Now()
	RunWithTimeout(5*time.Millisecond, slow)
	require.GreaterOrEqual(t, time.Since(startAt), 5*time.Millisecond)
	require.Less(t, time.Since(startAt), 10*time.Millisecond)
}

func ExampleRace() {
	startAt := time.Now()
	Race(
		func() { time.Sleep(time.Millisecond) },
		func() { time.Sleep(time.Second) },
		func() { time.Sleep(time.Minute) },
	)

	fmt.Println(time.Since(startAt) < time.Second)
	// Output:
	// true

}

func TestRace(t *testing.T) {
	startAt := time.Now()
	Race(
		func() { time.Sleep(time.Millisecond) },
		func() { time.Sleep(time.Second) },
		func() { time.Sleep(time.Minute) },
	)

	require.GreaterOrEqual(t, time.Since(startAt), time.Millisecond)
	require.Less(t, time.Since(startAt), time.Second)
}
