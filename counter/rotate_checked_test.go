package counter

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRotateCounterCheckedErrors verifies errors are distinguishable without changing state.
func TestRotateCounterCheckedErrors(t *testing.T) {
	for _, initial := range []struct{ n, point int64 }{{0, 0}, {-1, 1}, {1, 1}, {0, -1}} {
		_, err := NewRotateCounterFromN(initial.n, initial.point)
		require.Error(t, err)
	}
	_, err := NewRotateCounterWithCtx(nil, 10)
	require.Error(t, err)
	var zero RotateCounter
	_, err = zero.CountNChecked(1)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	c, err := NewRotateCounterFromNWithCtx(ctx, 3, 10)
	require.NoError(t, err)
	_, err = c.CountNChecked(-1)
	require.Error(t, err)
	require.Equal(t, int64(3), c.Get())
	cancel()
	_, err = c.CountNChecked(1)
	require.ErrorIs(t, err, context.Canceled)
	c.Close()
	_, err = c.CountNChecked(1)
	require.ErrorIs(t, err, ErrRotateCounterClosed)
	n, err := c.CountNChecked(0)
	require.NoError(t, err)
	require.Equal(t, int64(3), n)
}

// FuzzRotateCounterArithmetic compares constant-time stepping to an independent arbitrary-precision oracle.
func FuzzRotateCounterArithmetic(f *testing.F) {
	f.Add(int64(7), int64(10), int64(3))
	f.Add(int64(0), int64(1), int64(9223372036854775807))
	f.Fuzz(func(t *testing.T, start, point, step int64) {
		if point <= 0 || start < 0 || start >= point || step <= 0 {
			t.Skip()
		}
		c, err := NewRotateCounterFromN(start, point)
		require.NoError(t, err)
		expected := new(big.Int).Add(big.NewInt(start), big.NewInt(step))
		expected.Mod(expected, big.NewInt(point))
		if expected.Sign() == 0 {
			expected.SetInt64(point)
		}
		require.Equal(t, expected.Int64(), c.CountN(step))
		require.Equal(t, expected.Int64(), c.Get())
	})
}
