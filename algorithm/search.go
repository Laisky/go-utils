package algorithm

// BinarySearch searches for target in a sorted slice s.
//
// cmp must implement the same ordering as the slice,
// i.e. it must return a negative value if the target is less than the element at index,
// a positive value if the target is greater than the element at index,
// and zero if the target is equal to the element at index.
//
// Returns the index of target in s, or -1 if target is not present.
//
// I thinks this function is better than built-in slices.BinarySearchFunc,
// since of the cmp function is more flexible, you can get the index and element
// at each iteration, that can enpower you to do more things rather than just comparing.
// BTW, I think there is no need for the target as the parameter,
// since you can wrap the target in the cmp function.
func BinarySearch[T any](s []T, cmp func(index int, element T) int) int {
	leftIdx, rightIdx := 0, len(s)-1

	for leftIdx <= rightIdx {
		midIdx := leftIdx + (rightIdx-leftIdx)/2
		res := cmp(midIdx, s[midIdx])

		if res == 0 {
			return midIdx
		} else if res < 0 {
			rightIdx = midIdx - 1
		} else {
			leftIdx = midIdx + 1
		}
	}

	return -1
}
