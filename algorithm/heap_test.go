package algorithm

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetLargestNItems verifies that GetLargestNItems consumes a channel of 100000 random ints
// with five larger values interleaved and returns exactly those five values in descending order.
func TestGetLargestNItems(t *testing.T) {
	t.Parallel()

	expected := []int{
		1000009,
		1000008,
		1000007,
		1000006,
		1000005,
	}

	inputChan := make(chan int)

	go func() {
		defer close(inputChan)
		var idx int
		for i := 0; i < 100000; i++ {
			inputChan <- rand.Intn(100000)
			if idx != len(expected) && rand.Intn(1000) == 1 {
				inputChan <- expected[idx]
				idx++
			}
		}

		for _, it := range expected[idx:] {
			inputChan <- it
		}
	}()

	result, err := GetLargestNItems(inputChan, 5)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

// TestGetSmallestNItems verifies that GetSmallestNItems consumes a channel of 100000 random ints
// with five smaller values interleaved and returns exactly those five values in ascending order.
func TestGetSmallestNItems(t *testing.T) {
	t.Parallel()

	expected := []int{
		0, 1, 2, 3, 4,
	}

	inputChan := make(chan int)

	go func() {
		defer close(inputChan)
		var idx int
		for i := 0; i < 100000; i++ {
			inputChan <- rand.Intn(10000) + 100
			if idx != len(expected) && rand.Intn(1000) == 1 {
				inputChan <- expected[idx]
				idx++
			}
		}

		for _, it := range expected[idx:] {
			inputChan <- it
		}
	}()

	result, err := GetSmallestNItems(inputChan, 5)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
