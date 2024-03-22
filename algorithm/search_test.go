package algorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBinarySearch(t *testing.T) {
	t.Parallel()

	t.Run("TestBinarySearch_existingElement", func(t *testing.T) {
		slice := []int{1, 2, 3, 4, 5}
		target := 3
		expected := 2
		result := BinarySearch(slice, func(index int, element int) int {
			if element == target {
				return 0
			} else if element < target {
				return -1
			} else {
				return 1
			}
		})
		assert.Equal(t, expected, result, "Expected index %d, but got %d", expected, result)
	})

	t.Run("TestBinarySearch_nonExistingElement", func(t *testing.T) {
		slice := []int{1, 2, 3, 4, 5}
		target := 6
		expected := -1
		result := BinarySearch(slice, func(index int, element int) int {
			return target - element
		})
		assert.Equal(t, expected, result, "Expected index %d, but got %d", expected, result)
	})

	t.Run("TestBinarySearch_existingElementString", func(t *testing.T) {
		slice := []string{"apple", "banana", "cherry", "date", "elderberry"}
		target := "cherry"
		expected := 2
		result := BinarySearch(slice, func(index int, element string) int {
			if element == target {
				return 0
			} else if element < target {
				return -1
			} else {
				return 1
			}
		})
		assert.Equal(t, expected, result, "Expected index %d, but got %d", expected, result)
	})
}
