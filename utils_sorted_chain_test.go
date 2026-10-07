package utils

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/common"
)

// TestCombineSortedChain verifies that CombineSortedChain rejects an empty channel list and merges one or more
// sorted input channels into a single output that stays sorted in ascending or descending order, including
// inputs with disjoint ranges and continuously overlapping random streams.
func TestCombineSortedChain(t *testing.T) {
	t.Parallel()

	t.Run("Empty input channels", func(t *testing.T) {
		t.Parallel()
		_, err := CombineSortedChain[int](common.SortOrderAsc)
		require.Error(t, err)
		require.Equal(t, "chans cannot be empty", err.Error())
	})

	t.Run("single input channel", func(t *testing.T) {
		t.Parallel()
		ch := make(chan int)
		go func() {
			defer close(ch)
			ch <- 1
		}()

		result, err := CombineSortedChain(common.SortOrderAsc, ch)
		require.NoError(t, err)
		require.Equal(t, 1, <-result)
	})

	t.Run("multiple input channels", func(t *testing.T) {
		t.Parallel()
		ch1 := make(chan int)
		ch2 := make(chan int)
		ch3 := make(chan int)

		go func() {
			defer close(ch1)
			ch1 <- 1
		}()

		go func() {
			defer close(ch2)
			ch2 <- 3
		}()

		go func() {
			defer close(ch3)
			ch3 <- 2
		}()

		result, err := CombineSortedChain(common.SortOrderAsc, ch1, ch2, ch3)
		require.NoError(t, err)
		require.Equal(t, 1, <-result)
		require.Equal(t, 2, <-result)
		require.Equal(t, 3, <-result)
	})

	t.Run("multiple asc input channels with huge difference", func(t *testing.T) {
		t.Parallel()
		ch1 := make(chan int)
		ch2 := make(chan int)
		ch3 := make(chan int)

		go func() {
			defer close(ch1)
			for i := 0; i < 1000; i++ {
				ch1 <- i
			}
		}()

		go func() {
			defer close(ch2)
			for i := 0; i < 1000; i++ {
				ch2 <- i + 10000
			}
		}()

		go func() {
			defer close(ch3)
			for i := 0; i < 1000; i++ {
				ch3 <- i + 20000
			}
		}()

		result, err := CombineSortedChain(common.SortOrderAsc, ch1, ch2, ch3)
		require.NoError(t, err)
		latest, ok := <-result
		require.True(t, ok)

		for {
			cur, ok := <-result
			if !ok {
				return
			}

			require.Greater(t, cur, latest)
			latest = cur
		}
	})

	t.Run("multiple asc input channels with overlay", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch1 := make(chan int)
		ch2 := make(chan int)
		ch3 := make(chan int)

		go func() {
			defer close(ch1)
			i := rand.Intn(10000)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				ch1 <- i
				i += rand.Intn(1000)
			}
		}()

		go func() {
			defer close(ch2)
			i := rand.Intn(10000)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				ch2 <- i
				i += rand.Intn(1000)
			}
		}()

		go func() {
			defer close(ch3)
			i := rand.Intn(10000)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				ch3 <- i
				i += rand.Intn(1000)
			}
		}()

		result, err := CombineSortedChain(common.SortOrderAsc, ch1, ch2, ch3)
		require.NoError(t, err)

		latest, ok := <-result
		require.True(t, ok)

		for i := 0; i < 100000; i++ {
			cur, ok := <-result
			if !ok {
				return
			}

			require.LessOrEqualf(t, latest, cur, "%d", i)
			latest = cur
		}
	})

	t.Run("multiple desc input channels with huge difference", func(t *testing.T) {
		t.Parallel()
		ch1 := make(chan int)
		ch2 := make(chan int)
		ch3 := make(chan int)

		go func() {
			defer close(ch1)
			for i := 0; i < 1000; i++ {
				ch1 <- 10000 - i
			}
		}()

		go func() {
			defer close(ch2)
			for i := 0; i < 1000; i++ {
				ch2 <- 20000 - i
			}
		}()

		go func() {
			defer close(ch3)
			for i := 0; i < 1000; i++ {
				ch3 <- 30000 - i
			}
		}()

		result, err := CombineSortedChain(common.SortOrderDesc, ch1, ch2, ch3)
		require.NoError(t, err)
		latest, ok := <-result
		require.True(t, ok)

		for {
			cur, ok := <-result
			if !ok {
				return
			}

			require.Less(t, cur, latest)
			latest = cur
		}
	})
}

// Benchmark_CombineSortedChain measures receiving merged values from CombineSortedChain over three endless,
// randomly increasing ascending input channels, checking that the output stays ordered.
//
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_CombineSortedChain
// Benchmark_CombineSortedChain/CombineSortedChain
// Benchmark_CombineSortedChain/CombineSortedChain-104         	   52828	     23706 ns/op	      56 B/op	       3 allocs/op
func Benchmark_CombineSortedChain(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := make(chan int)
	ch2 := make(chan int)
	ch3 := make(chan int)

	go func() {
		defer close(ch1)
		i := rand.Intn(10000)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ch1 <- i
			i += rand.Intn(1000)
		}
	}()

	go func() {
		defer close(ch2)
		i := rand.Intn(10000)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ch2 <- i
			i += rand.Intn(1000)
		}
	}()

	go func() {
		defer close(ch3)
		i := rand.Intn(10000)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ch3 <- i
			i += rand.Intn(1000)
		}
	}()

	result, err := CombineSortedChain(common.SortOrderAsc, ch1, ch2, ch3)
	require.NoError(b, err)

	b.ResetTimer()
	b.Run("CombineSortedChain", func(b *testing.B) {
		latest, ok := <-result
		require.True(b, ok)

		for i := 0; i < b.N; i++ {
			cur, ok := <-result
			if !ok {
				return
			}

			require.LessOrEqual(b, latest, cur)
			latest = cur
		}
	})
}
