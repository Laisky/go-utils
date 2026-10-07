package algorithm

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewSkiplist verifies that 1000 random float64 keys stored with Set in a skiplist created
// by NewSkiplist can each be read back with Get, skipping keys that are already present.
func TestNewSkiplist(t *testing.T) {
	t.Parallel()

	l := NewSkiplist[float64]()

	var keys []float64
	for i := 0; i < 1000; i++ {
		k := rand.Float64()
		if v := l.Get(k); v != nil {
			// do not overwrite
			continue
		}

		l.Set(k, k)
		keys = append(keys, k)
	}

	for i, k := range keys {
		require.Equal(t, k, l.Get(k).Value().(float64), strconv.Itoa(i))
	}
}
