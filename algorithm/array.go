package algorithm

// DiffArray represents a difference array for efficient range updates.
type DiffArray struct {
	diff []int
}

// NewDiffArray initializes a DiffArray based on the input array.
func NewDiffArray(arr []int) *DiffArray {
	n := len(arr)
	diff := make([]int, n)

	if n > 0 {
		diff[0] = arr[0]
		for i := 1; i < n; i++ {
			diff[i] = arr[i] - arr[i-1]
		}
	}

	return &DiffArray{diff: diff}
}

// IncrementRange increments all elements in the range [l, r] by val.
func (d *DiffArray) IncrementRange(l, r, val int) {
	if l < 0 || r >= len(d.diff) || l > r {
		return
	}

	d.diff[l] += val
	if r+1 < len(d.diff) {
		d.diff[r+1] -= val
	}
}

// ToArray reconstructs and returns the updated array after all range increments.
func (d *DiffArray) ToArray() []int {
	n := len(d.diff)
	result := make([]int, n)

	if n > 0 {
		result[0] = d.diff[0]
		for i := 1; i < n; i++ {
			result[i] = result[i-1] + d.diff[i]
		}
	}

	return result
}
