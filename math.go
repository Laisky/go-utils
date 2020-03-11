package utils

import "math"

// Round Golang does not include a round function in the standard math package
// Round(123.555555, .5, 3) -> 123.556
func Round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

// MaxInt get the max int of two
func MaxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// MinInt get the min int of two
func MinInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}
