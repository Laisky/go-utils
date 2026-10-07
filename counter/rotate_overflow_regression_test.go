package counter

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// rotateReference computes the expected next counter value with arbitrary
// precision. It takes the current value, the positive step and the rotation
// point, and returns ((current + step) mod rotatePoint), mapping zero to the
// inclusive upper endpoint rotatePoint exactly like RotateCounter documents.
func rotateReference(current, step, rotatePoint int64) int64 {
	sum := new(big.Int).Add(big.NewInt(current), big.NewInt(step))
	next := sum.Mod(sum, big.NewInt(rotatePoint)).Int64()
	if next == 0 {
		return rotatePoint
	}
	return next
}

// TestRotateCounterBoundaryArithmeticMatchesReference verifies that the
// rotation arithmetic never overflows or truncates for extreme int64 inputs. It
// guards the gosec G115 lint cleanup in CountNChecked: every combination of
// boundary rotation points, initial values and steps must match a big.Int model.
func TestRotateCounterBoundaryArithmeticMatchesReference(t *testing.T) {
	t.Parallel()
	rotatePoints := []int64{1, 2, 3, 1 << 62, 1<<62 + 1, math.MaxInt64 - 1, math.MaxInt64}
	for _, rotatePoint := range rotatePoints {
		starts := []int64{0, 1, rotatePoint / 2, rotatePoint - 1}
		steps := []int64{
			1, 2, rotatePoint - 1, rotatePoint, rotatePoint + 1,
			math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64,
		}
		for _, start := range starts {
			if start < 0 || start >= rotatePoint {
				continue
			}
			for _, step := range steps {
				if step <= 0 {
					continue
				}
				counter, err := NewRotateCounterFromN(start, rotatePoint)
				require.NoError(t, err)
				expected := start
				// Two consecutive steps also cover the inclusive endpoint state
				// (current == rotatePoint) as the starting value of an increment.
				for range 2 {
					expected = rotateReference(expected, step, rotatePoint)
					got, err := counter.CountNChecked(step)
					require.NoError(t, err)
					require.Equal(t, expected, got,
						"rotatePoint=%d start=%d step=%d", rotatePoint, start, step)
					require.GreaterOrEqual(t, got, int64(1))
					require.LessOrEqual(t, got, rotatePoint)
					require.Equal(t, got, counter.Get())
				}
			}
		}
	}
}

// TestRotateCounterRandomArithmeticMatchesReference cross-checks random large
// rotation points and steps against the big.Int model with a fixed seed so any
// failure is reproducible. It complements the boundary test above.
func TestRotateCounterRandomArithmeticMatchesReference(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(20261007, 115))
	for range 2000 {
		rotatePoint := rng.Int64N(math.MaxInt64) + 1
		start := rng.Int64N(rotatePoint)
		counter, err := NewRotateCounterFromN(start, rotatePoint)
		require.NoError(t, err)
		expected := start
		for range 4 {
			step := rng.Int64N(math.MaxInt64) + 1
			expected = rotateReference(expected, step, rotatePoint)
			got, err := counter.CountNChecked(step)
			require.NoError(t, err)
			require.Equal(t, expected, got, "rotatePoint=%d step=%d", rotatePoint, step)
		}
	}
}
