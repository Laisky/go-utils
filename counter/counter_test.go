package counter

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

// ExampleCounter demonstrates creating a Counter, incrementing it by one with Count and by an
// arbitrary step with CountN, and reading the current total with Get.
func ExampleCounter() {
	counter := NewCounter()
	counter.Count()
	counter.CountN(10)
	counter.Get() // get current count
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

// TestCounter verifies that 10 goroutines each calling Count 1000 times on a shared Counter
// produce an exact total of 10000, and that Set overwrites the stored value.
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

// TestUint32Counter verifies that 10 goroutines each calling Count 1000 times on a shared
// Uint32Counter produce an exact total of 10000, and that Set overwrites the stored value.
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
