package common

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SortOrder sort order
type SortOrder int

const (
	// SortOrderAsc asc
	SortOrderAsc SortOrder = iota
	// SortOrderDesc desc
	SortOrderDesc
)

// Number is a number type
type Number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

// Sortable Data types that can be compared by >, <, ==
type Sortable interface {
	Number | string
}

// Min return the minimal value
func Min[T Sortable](vals ...T) T {
	if len(vals) == 0 {
		panic("empty vals")
	}

	min := vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
	}

	return min
}

// Max return the maximal value
func Max[T Sortable](vals ...T) T {
	if len(vals) == 0 {
		panic("empty vals")
	}

	max := vals[0]
	for _, v := range vals {
		if v > max {
			max = v
		}
	}

	return max
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
//   - bytes:
//   - si: `si ? 1024 : 1000`
//
// Example:
//
//	`HumanReadableByteCount(1005, false) -> "1.01KB"`
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

// type isc[T Sortable] struct {
// 	vals  []T
// 	chans []chan T
// }

// type iscSortItem struct {
// 	idx int
// }

// func newISC[T Sortable](chans ...chan T) (*isc[T], error) {
// 	v := &isc[T]{
// 		chans: chans,
// 		vals:  make([]T, len(chans)),
// 	}

// 	return v, nil
// }

// // updateVal update specific cursor val by channs,
// // if idx < 0, update all vals
// func (c isc[T]) updateVal(idx int) (closed bool) {
// 	if c.chans[idx] == nil {
// 		return true
// 	}

// 	if idx < 0 {
// 		for i := range c.chans {
// 			if c.updateVal(i) {
// 				return true
// 			}
// 		}

// 		return false
// 	}

// 	v, ok := <-c.chans[idx]
// 	if !ok {
// 		c.chans[idx] = nil
// 		return true
// 	}

// 	c.vals[idx] = v
// 	return false
// }

// func (c isc[T]) allEqual() (idx int, allEqual, allFinished bool) {
// 	allFinished = true
// 	var prevIdx int
// 	for idx := range c.chans {
// 		if c.chans[idx] == nil {
// 			continue
// 		}

// 		allFinished = false
// 		if prevIdx == 0 {
// 			prevIdx = idx
// 			continue
// 		}

// 		if c.vals[idx] != c.vals[prevIdx] {
// 			return prevIdx, false, allFinished
// 		}

// 		prevIdx = idx
// 	}

// 	return prevIdx, true, allFinished
// }

// func (c isc[T]) getSmallestTwo() (smallest, smaller iscSortItem) {
// 	smallest = iscSortItem{idx: -1}
// 	smaller = iscSortItem{idx: -1}
// 	for idx := range c.chans {
// 		if c.chans[idx] == nil {
// 			continue
// 		}

// 		v := c.vals[idx]
// 		if v < c.vals[smallest.idx] {
// 			smaller.idx = smallest.idx
// 			smallest.idx = idx
// 		} else if v < c.vals[smaller.idx] {
// 			smaller.idx = idx
// 		}
// 	}

// 	return
// }

// // IntersectSortedChans return the intersection of multiple sorted chans
// func IntersectSortedChans[T Sortable](chans ...chan T) (result chan T, err error) {
// 	if len(chans) < 2 {
// 		return nil, errors.Errorf("at least two chans required")
// 	}

// 	result = make(chan T)
// 	isc, err := newISC(chans...)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if isc.updateVal(-1) {
// 		return nil, errors.Errorf("some chan already closed")
// 	}

// 	go func() {
// 		defer close(result)
// 		for {
// 			if idx, allEqual, allFinished := isc.allEqual(); allFinished {
// 				return
// 			} else if allEqual {
// 				result <- isc.vals[idx]
// 				if isc.updateVal(-1) {
// 					return
// 				}

// 				continue
// 			}

// 			smallest, smaller := isc.getSmallestTwo()
// 			smallestV := isc.vals[smallest.idx]
// 			needUpdate := smaller.idx != -1 && smallestV == isc.vals[smaller.idx]
// 			for smallestV < isc.vals[smaller.idx] || needUpdate {
// 				if isc.updateVal(smallest.idx) {
// 					return
// 				}

// 				smallestV = isc.vals[smallest.idx]
// 				needUpdate = false
// 			}
// 		}
// 	}()

// 	return result, nil
// }

// Number2Roman convert number to roman
func Number2Roman(n int) (roman string) {
	if n < 1 || n > 3999 {
		return ""
	}

	for n > 0 {
		if n <= 12 {
			switch n {
			case 1:
				roman += "\u2160"
			case 2:
				roman += "\u2161"
			case 3:
				roman += "\u2162"
			case 4:
				roman += "\u2163"
			case 5:
				roman += "\u2164"
			case 6:
				roman += "\u2165"
			case 7:
				roman += "\u2166"
			case 8:
				roman += "\u2167"
			case 9:
				roman += "\u2168"
			case 10:
				roman += "\u2169"
			case 11:
				roman += "\u216A"
			case 12:
				roman += "\u216B"
			}
			return
		}

		switch {
		case n >= 1000:
			roman += "\u216F"
			n -= 1000
		case n >= 900:
			roman += "\u216D\u216F"
			n -= 900
		case n >= 500:
			roman += "\u216E"
			n -= 500
		case n >= 400:
			roman += "\u216D\u216E"
			n -= 400
		case n >= 100:
			roman += "\u216D"
			n -= 100
		case n >= 90:
			roman += "\u2169\u216D"
			n -= 90
		case n >= 50:
			roman += "\u216C"
			n -= 50
		case n >= 40:
			roman += "\u2169\u216C"
			n -= 40
		case n >= 10:
			roman += "\u2169"
			n -= 10
		case n >= 9:
			roman += "\u2168"
			n -= 9
		case n >= 5:
			roman += "\u2164"
			n -= 5
		case n >= 4:
			roman += "\u2163"
			n -= 4
		case n >= 1:
			roman += "\u2160"
			n--
		}
	}

	return roman
}
