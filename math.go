package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

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

// AbsInt64 abs(v)
//
// ignore int exceeds limit error, abs(MinInt64) == MaxInt64
func AbsInt64(v int64) int64 {
	switch {
	case v == math.MinInt64:
		return math.MaxInt64
	case v < 0:
		return -v
	default:
		return v
	}
}

// AbsInt32 abs(v)
//
// ignore int exceeds limit error, abs(MinInt32) == MaxInt32
func AbsInt32(v int32) int32 {
	switch {
	case v == math.MinInt32:
		return math.MaxInt32
	case v < 0:
		return -v
	default:
		return v
	}
}

// Round round float64
//
// Round(1.005, 2) -> 1.01
func Round(val float64, d int) float64 {
	n := math.Pow10(d)
	return math.Round(val*n+0.0001) / n
}

// HumanReadableByteCount convert bytes to human readable string
//
// Args:
//   * bytes:
//   * si: `si ? 1024 : 1000`
//
// Example:
//   `HumanReadableByteCount(1005, false) -> "1.01KB"`
func HumanReadableByteCount(bytes int64, si bool) (ret string) {
	var unit float64
	if si {
		unit = 1024
	} else {
		unit = 1000
	}

	if bytes < int64(unit) {
		return strconv.Itoa(int(bytes)) + "B"
	}

	unitChars := strings.Split("KMGTPE", "")
	for i := len(unitChars); i > 0; i-- {
		d := math.Pow(unit, float64(i))
		if bytes < int64(d) {
			continue
		}

		r := float64(bytes) / d
		switch {
		case r >= 1000:
			return "1" + unitChars[i] + "B"
		default:
			return strings.ReplaceAll(fmt.Sprintf("%.2f%sB", Round(r, 2), unitChars[i-1]), ".00", "")
		}
	}

	panic(fmt.Sprintf("unknown bytes `%v`", bytes))
}
