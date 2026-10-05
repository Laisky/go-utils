package counter

import (
	"context"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRotateCounterReportsOnlyConsumedValues verifies observation never reads prefetched state.
func TestRotateCounterReportsOnlyConsumedValues(t *testing.T) {
	counter, err := NewRotateCounterFromN(7, 1000003)
	require.NoError(t, err)
	defer counter.Close()
	// The legacy implementation has a producer queue. Wait for its real
	// prefetch work when running this same test against the unfixed source.
	// The corrected implementation has no producer; public invariants below
	// are asserted regardless of the internal representation.
	queue := reflect.ValueOf(counter).Elem().FieldByName("c")
	if queue.IsValid() && queue.Kind() == reflect.Chan {
		require.Eventually(t, func() bool { return queue.Len() > 0 }, time.Second, time.Millisecond)
	}
	require.Equal(t, int64(7), counter.Get())
	require.Equal(t, int64(7), counter.CountN(0))
	require.Equal(t, int64(8), counter.Count())
	require.Equal(t, int64(8), counter.Get())
	require.Equal(t, int64(11), counter.CountN(3))
	require.Equal(t, int64(11), counter.Get())
}

// TestRotateCounterWrapAndLargeSteps checks the retained 1..rotatePoint interval without signed overflow.
func TestRotateCounterWrapAndLargeSteps(t *testing.T) {
	counter, err := NewRotateCounter(10)
	require.NoError(t, err)
	defer counter.Close()
	require.Equal(t, int64(0), counter.Get())
	require.Equal(t, int64(10), counter.CountN(10))
	require.Equal(t, int64(1), counter.Count())
	require.Equal(t, int64(1), counter.CountN(10))
	require.Equal(t, int64(0), counter.CountN(-1))
	require.Equal(t, int64(1), counter.Get())
	done := make(chan int64, 1)
	go func() { done <- counter.CountN(math.MaxInt64) }()
	select {
	case got := <-done:
		require.Equal(t, int64(8), got)
	case <-time.After(2 * time.Second):
		t.Fatal("large CountN must finish in bounded time")
	}
	maxCounter, err := NewRotateCounterFromN(math.MaxInt64-1, math.MaxInt64)
	require.NoError(t, err)
	defer maxCounter.Close()
	require.Equal(t, int64(math.MaxInt64), maxCounter.Count())
	require.Equal(t, int64(1), maxCounter.Count())
	require.Equal(t, int64(1), maxCounter.CountN(math.MaxInt64))
}

// TestRotateCounterConcurrentConsumers verifies one linear sequence across independent consumers.
func TestRotateCounterConcurrentConsumers(t *testing.T) {
	counter, err := NewRotateCounter(math.MaxInt64)
	require.NoError(t, err)
	defer counter.Close()
	const workers, each = 8, 500
	results := make(chan int64, workers*each)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				results <- counter.Count()
			}
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[int64]bool, workers*each)
	for result := range results {
		require.GreaterOrEqual(t, result, int64(1))
		require.LessOrEqual(t, result, int64(workers*each))
		require.False(t, seen[result])
		seen[result] = true
	}
	require.Len(t, seen, workers*each)
	require.Equal(t, int64(workers*each), counter.Get())
}

// TestRotateCounterCloseAndCancellationNeverBlock verifies lifecycle operations are idempotent and bounded.
func TestRotateCounterCloseAndCancellationNeverBlock(t *testing.T) {
	counter, err := NewRotateCounter(100)
	require.NoError(t, err)
	require.Equal(t, int64(1), counter.Count())
	done := make(chan struct{})
	go func() {
		for range 100 {
			counter.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("repeated Close blocked")
	}
	require.Equal(t, int64(1), counter.Get())
	require.Equal(t, int64(1), counter.CountN(0))
	result := make(chan int64, 1)
	go func() { result <- counter.Count() }()
	select {
	case got := <-result:
		require.Zero(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("Count after Close blocked")
	}

	ctx, cancel := context.WithCancel(context.Background())
	canceled, err := NewRotateCounterWithCtx(ctx, 10)
	require.NoError(t, err)
	cancel()
	result = make(chan int64, 1)
	go func() { result <- canceled.Count() }()
	select {
	case got := <-result:
		require.Zero(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("Count after cancellation blocked")
	}
	canceled.Close()
}
