package utils

import "fmt"

// JumpHash fatest consistent hashing created by google.
// inspired by https://medium.com/@dgryski/consistent-hashing-algorithmic-tradeoffs-ef6b8e2fcae8
func JumpHash(key uint64, numBuckets int) (int32, error) {
	var b, j int64

	if numBuckets <= 0 {
		return 0, fmt.Errorf("numBuckets should greater than 0")
	}

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int32(b), nil
}
