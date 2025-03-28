package utils

import "github.com/Laisky/errors/v2"

// JumpHash fatest consistent hashing created by google.
// inspired by https://medium.com/@dgryski/consistent-hashing-algorithmic-tradeoffs-ef6b8e2fcae8
func JumpHash(key uint64, numBuckets int) (int32, error) {
	var b, j int64

	if numBuckets <= 0 {
		return 0, errors.Errorf("numBuckets should greater than 0")
	}

	if numBuckets > (1<<31)-1 {
		return 0, errors.Errorf("numBuckets should not exceed MAX_INT32 (%d)", (1<<31)-1)
	}

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	// b will always be less than numBuckets, which we've verified is within int32 range
	return int32(b), nil //nolint:gosec  //G115: integer overflow
}
