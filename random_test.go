package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRandomStringWithLength(t *testing.T) {
	for i := 0; i < 10; i++ {
		n, err := SecRandInt(1000)
		if err != nil {
			require.NoError(t, err)
		}

		ret := RandomStringWithLength(n)
		require.Len(t, ret, n)

		ret, err = SecRandomStringWithLength(n)
		require.NoError(t, err)
		require.Len(t, ret, n)
	}
}

func TestRandomBoundsValidation(t *testing.T) {
	t.Parallel()

	// Negative length should be rejected
	_, err := RandomBytesWithLength(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "length must be in")

	_, err = SecRandomBytesWithLength(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "length must be in")

	_, err = SecRandomStringWithLength(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "length must be in")

	// RandomStringWithLength returns empty for invalid input
	require.Equal(t, "", RandomStringWithLength(-1))
	require.Equal(t, "", RandomStringWithLength(maxRandomLength+1))

	// Exceeding max length should be rejected
	_, err = RandomBytesWithLength(maxRandomLength + 1)
	require.Error(t, err)

	_, err = SecRandomBytesWithLength(maxRandomLength + 1)
	require.Error(t, err)

	_, err = SecRandomStringWithLength(maxRandomLength + 1)
	require.Error(t, err)

	// Zero length should succeed
	b, err := RandomBytesWithLength(0)
	require.NoError(t, err)
	require.Len(t, b, 0)

	b, err = SecRandomBytesWithLength(0)
	require.NoError(t, err)
	require.Len(t, b, 0)

	s, err := SecRandomStringWithLength(0)
	require.NoError(t, err)
	require.Len(t, s, 0)

	require.Equal(t, "", RandomStringWithLength(0))
}

func TestSecRandIntValidation(t *testing.T) {
	t.Parallel()

	// Zero or negative upper bound should be rejected
	_, err := SecRandInt(0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upper bound must be positive")

	_, err = SecRandInt(-5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upper bound must be positive")

	// Positive values should work
	v, err := SecRandInt(10)
	require.NoError(t, err)
	require.True(t, v >= 0 && v < 10)
}

func TestRandomChoice(t *testing.T) {
	var arr []int64
	for i := 0; i < 10000; i++ {
		arr = append(arr, time.Now().UnixNano())
	}

	randor := NewRand()
	for i := 0; i < 1000; i++ {
		n := randor.Intn(10000)
		got := RandomChoice(arr, n)
		require.Len(t, got, n, "n: %d, got: %d", n, len(got))
	}
}

// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// BenchmarkRandomChoice/run-16         	    8062	    130228 ns/op	    7472 B/op	      10 allocs/op
func BenchmarkRandomChoice(b *testing.B) {
	var arr []int64
	for i := 0; i < 10000; i++ {
		arr = append(arr, time.Now().UnixNano())
	}

	b.Run("run", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			got := RandomChoice(arr, 100)
			if len(got) != 100 {
				b.FailNow()
			}
		}
	})
}
