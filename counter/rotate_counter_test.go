package counter

import (
	"context"
	"testing"
	"time"
)

func ExampleRotateCounter() {
	counter, err := NewRotateCounter(10)
	if err != nil {
		panic(err)
	}

	counter.Count()    // 1
	counter.CountN(10) // 1
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
