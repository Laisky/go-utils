package counter

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/go-utils/v6/log"
)

func ExampleCounter() {
	counter := NewCounter()
	counter.Count()
	counter.CountN(10)
	counter.Get() // get current count
}

func ExampleRotateCounter() {
	counter, err := NewRotateCounter(10)
	if err != nil {
		panic(err)
	}

	counter.Count()    // 1
	counter.CountN(10) // 1
}

// validateCounter hammers counter from parallel goroutines with Count and
// CountN and reports through t any value returned twice. N is the number of
// Count calls per goroutine, wg tracks the spawned goroutines, name labels
// failures, and store (shared when several children of one parallel counter
// must never overlap) records every value seen; nil allocates a private map.
func validateCounter(t *testing.T, N int, wg *sync.WaitGroup, counter Int64CounterItf, name string, store *sync.Map) {
	t.Helper()
	defer wg.Done()
	const nParallel = 10
	padding := struct{}{}
	if store == nil {
		store = &sync.Map{}
	}

	for range nParallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range N {
				n := counter.Count()
				if _, dup := store.LoadOrStore(n, padding); dup {
					t.Errorf("%s returned duplicate value %d from Count", name, n)
					return
				}
			}
		}()
	}
	for range nParallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range N / 100 {
				step := rand.Int63n(100) + 1
				n := counter.CountN(step)
				if _, dup := store.LoadOrStore(n, padding); dup {
					t.Errorf("%s returned duplicate value %d from CountN(%d)", name, n, step)
					return
				}
			}
		}()
	}
}

// TestCounterValidation verifies that every counter implementation returns
// unique values under concurrent Count and CountN calls.
func TestCounterValidation(t *testing.T) {
	// 20k calls per goroutine across 50 goroutines keeps heavy contention while
	// staying fast under the race detector.
	const callsPerGoroutine = 20000
	wg := &sync.WaitGroup{}
	atomicCounter := NewCounter()
	rotateCounter, err := NewRotateCounter(math.MaxInt64)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	parallelCounter, err := NewParallelCounter(100, math.MaxInt64)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	store2ChildCounter := &sync.Map{}
	childCounter1 := parallelCounter.GetChild()
	childCounter2 := parallelCounter.GetChild()
	childCounter3 := parallelCounter.GetChild()

	wg.Add(5)
	go validateCounter(t, callsPerGoroutine, wg, atomicCounter, "atomicCounter", nil)
	go validateCounter(t, callsPerGoroutine, wg, rotateCounter, "rotateCounter", nil)
	go validateCounter(t, callsPerGoroutine, wg, childCounter1, "childCounter-1", store2ChildCounter)
	go validateCounter(t, callsPerGoroutine, wg, childCounter2, "childCounter-2", store2ChildCounter)
	go validateCounter(t, callsPerGoroutine, wg, childCounter3, "childCounter-3", store2ChildCounter)
	wg.Wait()
}

func TestCounter(t *testing.T) {
	counter := NewCounterFromN(0)
	counter = NewCounter()
	wg := &sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				counter.Count()
			}
		}()
	}

	wg.Wait()
	if counter.Get() != 10000 {
		t.Errorf("expect 10000, got %v", counter.Get())
	}

	counter.Set(10)
	if counter.Get() != 10 {
		t.Errorf("expect 10, got %v", counter.Get())
	}
}

func TestUint32Counter(t *testing.T) {
	counter := NewUint32Counter()
	wg := &sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				counter.Count()
			}
		}()
	}

	wg.Wait()
	if counter.Get() != 10000 {
		t.Errorf("expect 10000, got %v", counter.Get())
	}

	counter.Set(10)
	if counter.Get() != 10 {
		t.Errorf("expect 10, got %v", counter.Get())
	}
}

func TestRotateCounter(t *testing.T) {
	_, err := NewRotateCounterFromN(100, 10)
	if err == nil {
		t.Fatal("should got error")
	}

	counter, err := NewRotateCounter(10)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var r int64
	if r = counter.Count(); r != 1 {
		t.Fatalf("want %v, got %v", 1, r)
	}
	if r = counter.Count(); r != 2 {
		t.Fatalf("want %v, got %v", 2, r)
	}
	if r = counter.CountN(3); r != 5 {
		t.Fatalf("want %v, got %v", 5, r)
	}
	if r = counter.CountN(10); r != 5 {
		t.Fatalf("want %v, got %v", 5, r)
	}
	if r = counter.CountN(248); r != 3 {
		t.Fatalf("want %v, got %v", 3, r)
	}
}

// TestParallelRotateCounter verifies rotation bounds of a parallel counter
// child and that two children never return the same value.
func TestParallelRotateCounter(t *testing.T) {
	pcounter, err := NewParallelCounter(10, 100)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	counter := pcounter.GetChild()

	var (
		start, got, step int64
	)
	if got = counter.Count(); got-start < 1 {
		t.Fatalf("%v should bigger than %v", got, start)
	}
	start = got

	if got = counter.Count(); got-start < 1 {
		t.Fatalf("%v should bigger than %v", got, start)
	}
	start = got

	if got = counter.Count(); got-start < 1 {
		t.Fatalf("%v should bigger than %v", got, start)
	}
	start = got

	step = 4
	if got = counter.CountN(step); got-start < step {
		t.Fatalf("%v should bigger than %v", got, start)
	}
	start = got

	step = 15
	if got = counter.CountN(step); got-start < step {
		t.Fatalf("%v should bigger than %v", got, start)
	}
	start = got

	step = 110
	if got = counter.CountN(step); got > step+start%100 {
		t.Fatalf("%v should bigger than %v", got, step+start%100)
	}

	// test duplicate
	if pcounter, err = NewParallelCounter(0, 10000000); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	counter1 := pcounter.GetChild()
	counter2 := pcounter.GetChild()

	var (
		ns     sync.Map
		wg     sync.WaitGroup
		failed atomic.Bool
		val    = struct{}{}
	)
	// record stores n and reports a duplicate through t; it returns false once
	// any goroutine has failed so both loops stop early.
	record := func(n int64) bool {
		if failed.Load() {
			return false
		}
		if _, dup := ns.LoadOrStore(n, val); dup {
			failed.Store(true)
			t.Errorf("parallel counter children returned duplicate value %d", n)
			return false
		}
		return true
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200000 {
			if !record(counter1.Count()) {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			if !record(counter2.CountN(100)) {
				return
			}
		}
	}()
	wg.Wait()
}

func TestRotateCounterFromN(t *testing.T) {
	counter, err := NewRotateCounterFromN(2, 10)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var r int64
	if r = counter.Count(); r != 3 {
		t.Fatalf("want %v, got %v", 3, r)
	}
	if r = counter.Count(); r != 4 {
		t.Fatalf("want %v, got %v", 4, r)
	}
	if r = counter.CountN(3); r != 7 {
		t.Fatalf("want %v, got %v", 7, r)
	}
	if r = counter.CountN(10); r != 7 {
		t.Fatalf("want %v, got %v", 7, r)
	}
}

func TestRotateCounterClose(t *testing.T) {
	t.Parallel()
	counter, err := NewRotateCounter(100)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	// Verify counter works
	r := counter.Count()
	if r < 1 {
		t.Fatalf("expected positive count, got %v", r)
	}

	// Close should not block (previously it sent on nil channel and blocked forever)
	done := make(chan struct{})
	go func() {
		counter.Close()
		close(done)
	}()

	select {
	case <-done:
		// success - Close returned
	case <-time.After(2 * time.Second):
		t.Fatal("Close() blocked - stopChan may be nil")
	}
}

func TestRotateCounterCloseWithCtx(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	counter, err := NewRotateCounterWithCtx(ctx, 100)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	// Verify counter works
	r := counter.Count()
	if r < 1 {
		t.Fatalf("expected positive count, got %v", r)
	}

	// Cancel context should also stop the rotator
	cancel()

	// Give time for goroutine to exit
	time.Sleep(100 * time.Millisecond)
	_ = counter
}

// BenchmarkCounter/count_1-8         	 1369930	       920 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/get_speed-8       	  620430	      2278 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_1_parallel_4-8         	  212336	      5285 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_5-8                    	18502207	        64.3 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_500-8                  	18213850	        64.1 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_500_parallel_4-8       	  239703	      5315 ns/op	       0 B/op	       0 allocs/op
func BenchmarkCounter(b *testing.B) {
	counter := NewCounter()

	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("get speed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.GetSpeed()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.CountN(500)
			}
		})
	})
}

func BenchmarkRotateCounter(b *testing.B) {
	counter, err := NewRotateCounter(1000000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.CountN(500)
			}
		})
	})
}

/*
BenchmarkAllCounter

✗ go test -run=All -bench=AllCo -benchtime=5s -benchmem
goos: darwin
goarch: amd64
pkg: github.com/Laisky/go-utils
BenchmarkAllCounter/atomicCounter_count_1-4             833836315                6.99 ns/op            0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_count_1-4             26496855               219 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_count_1-4              195491630               30.2 ns/op             1 B/op          0 allocs/op
BenchmarkAllCounter/atomicCounter_count_500-4           821179578                7.09 ns/op            0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_count_500-4              54483            108021 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_count_500-4              372174             15063 ns/op             960 B/op          5 allocs/op
BenchmarkAllCounter/atomicCounter_parallel-4_count_1-4          68061858               108 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_parallel-4_count_1-4           5469538              1221 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_parallel-4_count_1-4           30513360               211 ns/op               7 B/op          0 allocs/op
BenchmarkAllCounter/atomicCounter_parallel-4_count_500-4        63054807               107 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_parallel-4_count_500-4            9793            613852 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_parallel-4_count_500-4            60672            102970 ns/op            3840 B/op         20 allocs/op
PASS
ok      github.com/Laisky/go-utils      82.997s
*/
func BenchmarkAllCounter(b *testing.B) {
	b.ReportAllocs()
	var err error
	if err = log.Shared.ChangeLevel("info"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	atomicCounter := NewCounter()
	rotateCounter, err := NewRotateCounter(100000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	parallelCounter, err := NewParallelCounter(100, 100000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	childCounter := parallelCounter.GetChild()

	// count 1
	b.Run("atomicCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			atomicCounter.Count()
		}
	})
	b.Run("rotateCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rotateCounter.Count()
		}
	})
	b.Run("childCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			childCounter.Count()
		}
	})

	// count 500
	b.Run("atomicCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			atomicCounter.CountN(500)
		}
	})
	b.Run("rotateCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rotateCounter.CountN(500)
		}
	})
	b.Run("childCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			childCounter.CountN(500)
		}
	})

	// parallel count 1
	b.Run("atomicCounter parallel-4 count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
	})
	b.Run("rotateCounter parallel-4 count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
	})
	b.Run("childCounter parallel-4 count 1", func(b *testing.B) {
		cc1 := parallelCounter.GetChild()
		cc2 := parallelCounter.GetChild()
		cc3 := parallelCounter.GetChild()
		cc4 := parallelCounter.GetChild()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc1.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc2.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc3.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc4.Count()
			}
		})
	})

	// parallel count 500
	b.Run("atomicCounter parallel-4 count 500", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
	})
	b.Run("rotateCounter parallel-4 count 500", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
	})
	b.Run("childCounter parallel-4 count 500", func(b *testing.B) {
		cc1 := parallelCounter.GetChild()
		cc2 := parallelCounter.GetChild()
		cc3 := parallelCounter.GetChild()
		cc4 := parallelCounter.GetChild()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc1.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc2.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc3.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc4.CountN(500)
			}
		})
	})

}
