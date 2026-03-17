package utils

import "github.com/Laisky/errors/v2"

const (
	jumpHashMaxBuckets = (1 << 31) - 1
	jumpHashScale      = uint64(1) << 31
)

// JumpHash fatest consistent hashing created by google.
// inspired by https://medium.com/@dgryski/consistent-hashing-algorithmic-tradeoffs-ef6b8e2fcae8
func JumpHash(key uint64, numBuckets int) (int32, error) {
	var b, j int64

	if numBuckets <= 0 {
		return 0, errors.Errorf("numBuckets should greater than 0")
	}

	if numBuckets > jumpHashMaxBuckets {
		return 0, errors.Errorf("numBuckets should not exceed MAX_INT32 (%d)", jumpHashMaxBuckets)
	}

	scale := float64(jumpHashScale)

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		next := float64(b+1) * (scale / float64((key>>33)+1))
		j = int64(next)
	}

	// b will always be less than numBuckets, which we've verified is within int32 range
	return int32(b), nil //nolint:gosec  //G115: integer overflow
}
