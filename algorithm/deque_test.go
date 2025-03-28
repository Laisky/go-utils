package algorithm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeque(t *testing.T) {
	t.Run("test basic operations", func(t *testing.T) {
		q, err := NewDeque[int]()
		require.NoError(t, err)
		require.Equal(t, 0, q.Len())

		q.PushBack(1)
		q.PushBack(2)
		q.PushFront(3)
		require.Equal(t, 3, q.Len())

		require.Equal(t, 3, q.Front())
		require.Equal(t, 2, q.Back())

		val := q.PopFront()
		require.Equal(t, 3, val)
		require.Equal(t, 2, q.Len())

		val = q.PopBack()
		require.Equal(t, 2, val)
		require.Equal(t, 1, q.Len())

		val = q.PopFront()
		require.Equal(t, 1, val)
		require.Equal(t, 0, q.Len())
	})

	t.Run("test options", func(t *testing.T) {
		q, err := NewDeque[int](
			WithDequeMinimalCapacity(16),
			WithDequeCurrentCapacity(32),
		)
		require.NoError(t, err)
		require.Equal(t, 0, q.Len())

		_, err = NewDeque[int](WithDequeMinimalCapacity(-1))
		require.Error(t, err)

		_, err = NewDeque[int](WithDequeCurrentCapacity(-1))
		require.Error(t, err)
	})

	t.Run("test with different types", func(t *testing.T) {
		q, err := NewDeque[string]()
		require.NoError(t, err)

		q.PushBack("hello")
		q.PushBack("world")
		require.Equal(t, 2, q.Len())
		require.Equal(t, "hello", q.Front())
		require.Equal(t, "world", q.Back())
	})
}
