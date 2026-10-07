package utils

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-utils/v6/log"
)

// TestMutex verifies Mutex semantics: TryLock succeeds only when unlocked, TryRelease succeeds only when locked,
// SpinLock acquires a free lock at once and gives up after its 3s timeout when the lock is already held, and
// ForceRelease unlocks it.
func TestMutex(t *testing.T) {
	l := NewMutex()
	require.True(t, l.TryLock(), "should acquire lock")
	require.True(t, l.IsLocked(), "should locked")

	require.False(t, l.TryLock(), "should not acquire lock")
	require.True(t, l.TryRelease(), "should release lock")
	require.False(t, l.IsLocked(), "should not locked")
	require.False(t, l.TryRelease(), "should not release lock")
	l.SpinLock(1*time.Second, 3*time.Second)
	require.True(t, l.IsLocked(), "should locked")

	start := time.Now()
	l.SpinLock(1*time.Second, 3*time.Second)
	if time.Since(start) < 3*time.Second || time.Since(start) > 4100*time.Millisecond {
		t.Fatalf("duration: %v", time.Since(start).Seconds())
	}

	l.ForceRelease()
	require.False(t, l.IsLocked(), "should not locked")
}

// ExampleMutex demonstrates acquiring a Mutex with TryLock, returning early when it is already held, and
// releasing it with a deferred ForceRelease.
func ExampleMutex() {
	l := NewMutex()
	if !l.TryLock() {
		log.Shared.Info("can not acquire lock")
		return
	}
	defer l.ForceRelease()

}

// BenchmarkMutex measures parallel TryLock and TryRelease pairs on a single shared Mutex.
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

// TestNewExpiredRLock verifies that NewExpiredRLock succeeds and that the RWMutex returned by GetLock lets two
// read locks be taken and released while a writer goroutine is waiting on Lock.
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

// ExampleRunWithTimeout demonstrates that RunWithTimeout returns after its 5ms timeout instead of waiting for a
// function that sleeps for 10 seconds.
func ExampleRunWithTimeout() {
	slow := func() error {
		time.Sleep(10 * time.Second)
		return nil
	}
	startAt := time.Now()
	RunWithTimeout(5*time.Millisecond, slow)

	fmt.Println(time.Since(startAt) < 10*time.Second)
	// Output:
	// true
}

// TestRunWithTimeout verifies that RunWithTimeout with a 5ms timeout returns after at least 5ms but within 50ms
// when the wrapped function would take 10 seconds.
func TestRunWithTimeout(t *testing.T) {
	slow := func() error {
		time.Sleep(10 * time.Second)
		return nil
	}
	startAt := time.Now()
	RunWithTimeout(5*time.Millisecond, slow)
	require.GreaterOrEqual(t, time.Since(startAt), 5*time.Millisecond)
	require.Less(t, time.Since(startAt), 50*time.Millisecond)
}

// ExampleRaceErr demonstrates that RaceErr returns as soon as the fastest of three functions (1ms, 1s and 1min)
// finishes, well before one second has elapsed.
func ExampleRaceErr() {
	startAt := time.Now()
	_ = RaceErr(
		func() error {
			time.Sleep(time.Millisecond)
			return nil
		},
		func() error {
			time.Sleep(time.Second)
			return nil
		},
		func() error {
			time.Sleep(time.Minute)
			return nil
		},
	)

	fmt.Println(time.Since(startAt) < time.Second)
	// Output:
	// true

}

// TestRace verifies that RaceErr returns once the fastest of three functions (1ms) completes, taking at least
// 1ms and less than one second.
func TestRace(t *testing.T) {
	startAt := time.Now()
	_ = RaceErr(
		func() error {
			time.Sleep(time.Millisecond)
			return nil
		},
		func() error {
			time.Sleep(time.Second)
			return nil
		},
		func() error {
			time.Sleep(time.Minute)
			return nil
		},
	)

	require.GreaterOrEqual(t, time.Since(startAt), time.Millisecond)
	require.Less(t, time.Since(startAt), time.Second)
}

// TestRaceWithCtx verifies that RaceErrWithCtx returns the nil error of the fastest of three context-aware
// functions (1ms) after at least 1ms and less than one second.
func TestRaceWithCtx(t *testing.T) {
	t.Run("fatest task", func(t *testing.T) {
		startAt := time.Now()
		err := RaceErrWithCtx(
			context.Background(),
			func(context.Context) error {
				time.Sleep(time.Millisecond)
				return nil
			},
			func(context.Context) error {
				time.Sleep(time.Second)
				return nil
			},
			func(context.Context) error {
				time.Sleep(time.Minute)
				return nil
			},
		)
		require.NoError(t, err)

		require.GreaterOrEqual(t, time.Since(startAt), time.Millisecond)
		require.Less(t, time.Since(startAt), time.Second)
	})
}

// TestNewFlock verifies that a file lock whose path lies in a nonexistent directory fails both Lock and Unlock,
// and that two file locks on the same file in the same process can both Lock and then Unlock without error.
func TestNewFlock(t *testing.T) {
	dir, err := os.MkdirTemp("", "fs*")
	require.NoError(t, err)
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	lockfile := filepath.Join(dir, "test.lock")

	t.Run("file not exist", func(t *testing.T) {
		f, err := NewFlock("/123/" + lockfile)
		require.NoError(t, err)
		require.Error(t, f.Lock())
		require.Error(t, f.Unlock())
	})

	t.Run("same process", func(t *testing.T) {
		flock1, err := NewFlock(lockfile)
		require.NoError(t, err)
		flock2, err := NewFlock(lockfile)
		require.NoError(t, err)

		err = flock1.Lock()
		require.NoError(t, err)
		err = flock2.Lock()
		require.NoError(t, err)

		require.NoError(t, flock1.Unlock())
		require.NoError(t, flock2.Unlock())
	})
}

// TestRaceErrWithCtx verifies that RaceErrWithCtx returns a non-nil error when all 1000 racing functions fail
// after random delays of up to one second.
func TestRaceErrWithCtx(t *testing.T) {
	var gs []func(context.Context) error
	for i := 0; i < 1000; i++ {
		gs = append(gs, func(ctx context.Context) error {
			n := rand.Intn(1000)
			time.Sleep(time.Duration(n) * time.Millisecond)
			return errors.Errorf("%v", n)
		})
	}

	ctx := context.Background()
	err := RaceErrWithCtx(ctx, gs...)
	require.Error(t, err)
}

// TestRaceErr verifies that RaceErr returns a non-nil error when all 1000 racing functions fail after random
// delays of up to one second.
func TestRaceErr(t *testing.T) {
	var gs []func() error
	for i := 0; i < 1000; i++ {
		gs = append(gs, func() error {
			n := rand.Intn(1000)
			time.Sleep(time.Duration(n) * time.Millisecond)
			return errors.Errorf("%v", n)
		})
	}

	err := RaceErr(gs...)
	require.Error(t, err)
}

// TestRWManager_Lock verifies that a zero-value RWManager can concurrently acquire read locks and then write
// locks on 1000 distinct random names without blocking, and that every acquired lock can be released.
func TestRWManager_Lock(t *testing.T) {
	var m RWManager

	t.Run("rlock", func(t *testing.T) {
		var locks []string
		var pool errgroup.Group
		for i := 0; i < 1000; i++ {
			var lockname string
			for {
				lockname = RandomStringWithLength(10)
				if !Contains(locks, lockname) {
					locks = append(locks, lockname)
					break
				}
			}

			pool.Go(func() error {
				m.RLock(lockname)
				return nil
			})
		}

		require.NoError(t, pool.Wait())

		for _, lockname := range locks {
			m.RUnlock(lockname)
		}
	})

	t.Run("lock", func(t *testing.T) {
		var locks []string
		var pool errgroup.Group
		for i := 0; i < 1000; i++ {
			var lockname string
			for {
				lockname = RandomStringWithLength(10)
				if !Contains(locks, lockname) {
					locks = append(locks, lockname)
					break
				}
			}

			pool.Go(func() error {
				m.Lock(lockname)
				return nil
			})
		}

		require.NoError(t, pool.Wait())

		for _, lockname := range locks {
			m.Unlock(lockname)
		}
	})
}

// TestWaitComplete verifies that WaitComplete returns nil after all 1000 tasks finish when the context stays
// alive, and that it returns a context deadline error within one second, with only part of the tasks finished,
// when the context times out after 10ms.
func TestWaitComplete(t *testing.T) {
	t.Run("tasks finished before context cancel", func(t *testing.T) {
		var tasks []func(context.Context) error
		var n int
		var mu sync.Mutex
		for i := 0; i < 1000; i++ {
			tasks = append(tasks, func(ctx context.Context) error {
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

				mu.Lock()
				n++
				mu.Unlock()

				return nil
			})
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := WaitComplete(ctx, tasks...)
		require.NoError(t, err)
		require.Equal(t, 1000, n)
	})

	t.Run("tasks not finished before context cancel", func(t *testing.T) {
		var tasks []func(context.Context) error
		var n int
		var mu sync.Mutex
		for i := 0; i < 1000; i++ {
			tasks = append(tasks, func(ctx context.Context) error {
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

				mu.Lock()
				n++
				mu.Unlock()

				return nil
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		startat := time.Now()
		err := WaitComplete(ctx, tasks...)
		cost := time.Since(startat)

		require.ErrorContains(t, err, "context deadline exceeded")

		mu.Lock()
		require.Less(t, n, 1000)
		require.Greater(t, n, 0)
		mu.Unlock()

		require.GreaterOrEqual(t, cost, 10*time.Millisecond)
		require.Less(t, cost, time.Second)
	})
}

// BenchmarkRWManager_RLock measures parallel RLock/RUnlock on an RWMutex already stored in RWManager's map,
// comparing an unconditional LoadOrStore lookup with a Load-first lookup that falls back to LoadOrStore.
//
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// BenchmarkRWManager_RLock
// BenchmarkRWManager_RLock/LoadOrStore
// BenchmarkRWManager_RLock/LoadOrStore-16         	 4319887	       264.9 ns/op	      24 B/op	       1 allocs/op
// BenchmarkRWManager_RLock/LoadThenLoadOrStore
// BenchmarkRWManager_RLock/LoadThenLoadOrStore-16 	 5195000	       237.7 ns/op	       0 B/op	       0 allocs/op
// PASS
func BenchmarkRWManager_RLock(b *testing.B) {
	m := &RWManager{}
	name := "test"
	// Initialize the map to simulate a hot cache
	m.m.Store(name, &sync.RWMutex{})

	b.Run("LoadOrStore", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu, _ := m.m.LoadOrStore(name, &sync.RWMutex{})
				mu.(*sync.RWMutex).RLock()
				mu.(*sync.RWMutex).RUnlock()
			}
		})
	})

	b.Run("LoadThenLoadOrStore", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu, ok := m.m.Load(name)
				if !ok {
					mu, _ = m.m.LoadOrStore(name, &sync.RWMutex{})
				}
				mu.(*sync.RWMutex).RLock()
				mu.(*sync.RWMutex).RUnlock()
			}
		})
	})
}
