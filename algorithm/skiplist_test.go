package algorithm

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSkiplist(t *testing.T) {
	l := NewSkiplist()

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
