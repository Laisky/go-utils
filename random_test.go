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
