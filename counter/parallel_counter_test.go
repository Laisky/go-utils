package counter

import (
	"sync"
	"sync/atomic"
	"testing"
)

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
