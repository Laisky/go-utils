package algorithm

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fifoRetentionPayload is larger than a tiny allocation so its finalizer can
// independently observe reclamation of each dequeued payload.
type fifoRetentionPayload struct {
	id      int
	padding [4096]byte
}

// putFIFOTrackedPayload enqueues a separately allocated payload and counts its
// eventual reclamation without retaining the payload in the finalizer closure.
func putFIFOTrackedPayload(f *FIFO, id int, reclaimed *atomic.Int64) {
	payload := &fifoRetentionPayload{id: id}
	runtime.SetFinalizer(payload, func(*fifoRetentionPayload) {
		reclaimed.Add(1)
	})
	f.Put(payload)
}

// TestFIFOBoundedRetention verifies that a live, repeatedly drained queue does
// not retain its history, allowing only the current sentinel's last payload.
func TestFIFOBoundedRetention(t *testing.T) {
	f := NewFIFO()
	// Keep the queue live even if an assertion terminates this test early.
	defer func() { runtime.KeepAlive(f) }()
	var reclaimed atomic.Int64
	const rounds, batch = 3, 128
	for round := range rounds {
		for i := range batch {
			putFIFOTrackedPayload(f, round*batch+i, &reclaimed)
		}
		for i := range batch {
			payload, ok := f.Get().(*fifoRetentionPayload)
			require.True(t, ok)
			require.Equal(t, round*batch+i, payload.id)
		}
		require.Zero(t, f.Len())
		require.Nil(t, f.Get())
		require.Eventually(t, func() bool {
			runtime.GC()
			return reclaimed.Load() >= int64((round+1)*batch-1)
		}, 5*time.Second, 10*time.Millisecond,
			"a live queue must release all but its current sentinel payload")
	}
}

// TestFIFOCompatibility verifies empty results, nil payloads, FIFO order and
// quiescent length across repeated transitions between empty and nonempty.
func TestFIFOCompatibility(t *testing.T) {
	f := NewFIFO()
	var typedNil *int
	for range 10 {
		require.Nil(t, f.Get())
		values := []any{nil, typedNil, 0, "value"}
		for _, value := range values {
			f.Put(value)
		}
		require.Equal(t, len(values), f.Len())
		for i, value := range values {
			require.Equal(t, value, f.Get())
			require.Equal(t, len(values)-i-1, f.Len())
		}
		require.Nil(t, f.Get())
	}
}

// TestFIFOConcurrentExactlyOnce verifies that concurrent producers and consumers
// deliver every unique payload once while the garbage collector reclaims nodes.
func TestFIFOConcurrentExactlyOnce(t *testing.T) {
	const producers, consumers, perProducer = 8, 8, 1000
	const total = producers * perProducer
	f := NewFIFO()
	var writers, readers sync.WaitGroup
	var seen [total]atomic.Int32
	var invalid atomic.Int32
	start, done, stopGC := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var collector sync.WaitGroup
	collector.Go(func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopGC:
				return
			case <-ticker.C:
				runtime.GC()
			}
		}
	})
	for producer := range producers {
		writers.Go(func() {
			<-start
			for i := range perProducer {
				f.Put(producer*perProducer + i)
			}
		})
	}
	for range consumers {
		readers.Go(func() {
			<-start
			for {
				value := f.Get()
				if value == nil {
					select {
					case <-done:
						// Retry after observing producer completion: the previous
						// empty read may have preceded their last enqueue.
						value = f.Get()
						if value == nil {
							return
						}
					default:
						runtime.Gosched()
						continue
					}
				}
				id, ok := value.(int)
				if !ok || id < 0 || id >= total {
					invalid.Add(1)
					continue
				}
				seen[id].Add(1)
			}
		})
	}
	close(start)
	writers.Wait()
	close(done)
	readers.Wait()
	close(stopGC)
	collector.Wait()
	require.Zero(t, invalid.Load())
	for id := range total {
		require.Equal(t, int32(1), seen[id].Load(), "payload %d", id)
	}
	require.Zero(t, f.Len())
	require.Nil(t, f.Get())
}
