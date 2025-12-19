package algorithm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDiffArray(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "empty array",
			input:    []int{},
			expected: []int{},
		},
		{
			name:     "single element",
			input:    []int{5},
			expected: []int{5},
		},
		{
			name:     "ascending array",
			input:    []int{1, 2, 3, 4, 5},
			expected: []int{1, 1, 1, 1, 1},
		},
		{
			name:     "descending array",
			input:    []int{5, 4, 3, 2, 1},
			expected: []int{5, -1, -1, -1, -1},
		},
		{
			name:     "mixed values",
			input:    []int{3, 5, 2, 8, 1},
			expected: []int{3, 2, -3, 6, -7},
		},
		{
			name:     "all same values",
			input:    []int{7, 7, 7, 7},
			expected: []int{7, 0, 0, 0},
		},
		{
			name:     "negative values",
			input:    []int{-5, -3, -8, -1},
			expected: []int{-5, 2, -5, 7},
		},
		{
			name:     "mixed positive and negative",
			input:    []int{-2, 3, -1, 4},
			expected: []int{-2, 5, -4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffArray := NewDiffArray(tt.input)

			require.Equal(t, len(tt.expected), len(diffArray.diff), "expected diff length %d, got %d", len(tt.expected), len(diffArray.diff))
			require.Equal(t, tt.expected, diffArray.diff, "diff array mismatch")

			// Verify ToArray reconstructs original array
			reconstructed := diffArray.ToArray()
			require.Equal(t, len(tt.input), len(reconstructed), "reconstructed array length mismatch: expected %d, got %d", len(tt.input), len(reconstructed))
			require.Equal(t, tt.input, reconstructed, "reconstructed array mismatch")
		})
	}
}

func TestDiffArray_IncrementRange(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		ops   []struct {
			l, r, val int
		}
		expected []int
	}{
		{
			name:     "single increment whole array",
			input:    []int{1, 2, 3, 4, 5},
			ops:      []struct{ l, r, val int }{{0, 4, 2}},
			expected: []int{3, 4, 5, 6, 7},
		},
		{
			name:     "increment subrange",
			input:    []int{1, 2, 3, 4, 5},
			ops:      []struct{ l, r, val int }{{1, 3, 10}},
			expected: []int{1, 12, 13, 14, 5},
		},
		{
			name:     "multiple increments",
			input:    []int{0, 0, 0, 0},
			ops:      []struct{ l, r, val int }{{0, 1, 1}, {2, 3, 2}},
			expected: []int{1, 1, 2, 2},
		},
		{
			name:     "increment single element",
			input:    []int{5, 5, 5},
			ops:      []struct{ l, r, val int }{{1, 1, 7}},
			expected: []int{5, 12, 5},
		},
		{
			name:     "out of bounds (ignored)",
			input:    []int{1, 2, 3},
			ops:      []struct{ l, r, val int }{{-1, 2, 5}, {0, 5, 3}, {2, 1, 2}},
			expected: []int{1, 2, 3},
		},
		{
			name:     "empty array",
			input:    []int{},
			ops:      []struct{ l, r, val int }{{0, 0, 1}},
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			da := NewDiffArray(tt.input)
			for _, op := range tt.ops {
				da.IncrementRange(op.l, op.r, op.val)
			}
			result := da.ToArray()
			require.Equal(t, tt.expected, result, "after IncrementRange, array mismatch")
		})
	}
}
