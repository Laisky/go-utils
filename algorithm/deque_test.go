package algorithm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// -------------------------------------
// Heap
// -------------------------------------

// -------------------------------------
// FIFO
// -------------------------------------

func TestNewDeque(t *testing.T) {
	t.Run("err", func(t *testing.T) {
		var err error
		_, err = NewDeque[int](WithDequeCurrentCapacity(-1))
		require.Error(t, err)

		_, err = NewDeque[int](WithDequeMinimalCapacity(-1))
		require.Error(t, err)
	})

	d, err := NewDeque[int](
		WithDequeCurrentCapacity(0),
		WithDequeMinimalCapacity(0),
	)
	require.NoError(t, err)

	d.PushBack(10)
	d.PushFront(20)
	require.Equal(t, 2, d.Len())
	require.Equal(t, 20, d.PopFront())
	require.Equal(t, 10, d.PopBack())
}
