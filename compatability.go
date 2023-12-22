package utils

import "github.com/Laisky/go-utils/v4/common"

// Number is a number type
type Number common.Number

// Sortable Data types that can be compared by >, <, ==
type Sortable common.Sortable

var (
	// AbsInt64 abs(v)
	//
	// ignore int exceeds limit error, abs(MinInt64) == MaxInt64
	AbsInt64 = common.AbsInt64
	// AbsInt32 abs(v)
	//
	// ignore int exceeds limit error, abs(MinInt32) == MaxInt32
	AbsInt32 = common.AbsInt32
	// Round round float64
	//
	// Round(1.005, 2) -> 1.01
	Round = common.Round
	// HumanReadableByteCount convert bytes to human readable string
	//
	// Args:
	//   - bytes:
	//   - si: `si ? 1024 : 1000`
	//
	// Example:
	//
	//	`HumanReadableByteCount(1005, false) -> "1.01KB"`
	HumanReadableByteCount = common.HumanReadableByteCount
)

// Min return the minimal value
func Min[T Sortable](vals ...T) T { return common.Min(vals...) }

// Max return the maximal value
func Max[T Sortable](vals ...T) T { return common.Max(vals...) }
