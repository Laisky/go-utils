package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Laisky/errors"
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

type isc[T Sortable] struct {
	vals  []T
	chans []chan T
}

type iscSortItem struct {
	idx int
}

func newISC[T Sortable](chans ...chan T) (*isc[T], error) {
	v := &isc[T]{
		chans: chans,
		vals:  make([]T, len(chans)),
	}

	return v, nil
}

// updateVal update specific cursor val by channs,
// if idx < 0, update all vals
func (c isc[T]) updateVal(idx int) (closed bool) {
	if idx < 0 {
		for i := range c.chans {
			if c.updateVal(i) {
				return true
			}
		}

		return false
	}

	v, ok := <-c.chans[idx]
	if !ok {
		return true
	}

	c.vals[idx] = v
	return false
}

func (c isc[T]) allEqual() bool {
	return Max(c.vals...) == Min(c.vals...)
}

func (c isc[T]) getSmallestTwo() (smallest, smaller iscSortItem) {
	smallest = iscSortItem{idx: 0}
	smaller = iscSortItem{idx: 1}
	for idx, v := range c.vals {
		if v < c.vals[smallest.idx] {
			smaller.idx = smallest.idx
			smallest.idx = idx
		} else if v < c.vals[smaller.idx] {
			smaller.idx = idx
		}
	}

	return
}

// IntersectSortedChans return the intersection of multiple sorted chans
func IntersectSortedChans[T Sortable](chans ...chan T) (result chan T, err error) {
	if len(chans) < 2 {
		return nil, errors.Errorf("at least two chans required")
	}

	result = make(chan T)
	isc, err := newISC(chans...)
	if err != nil {
		return nil, err
	}

	if isc.updateVal(-1) {
		return nil, errors.Errorf("some chan already closed")
	}

	go func() {
		defer close(result)
		for {
			if isc.allEqual() {
				result <- isc.vals[0]
				if isc.updateVal(-1) {
					return
				}

				continue
			}

			smallest, smaller := isc.getSmallestTwo()
			smallestV := isc.vals[smallest.idx]
			needUpdate := smallestV == isc.vals[smaller.idx]
			for smallestV < isc.vals[smaller.idx] || needUpdate {
				if isc.updateVal(smallest.idx) {
					return
				}

				smallestV = isc.vals[smallest.idx]
				needUpdate = false
			}
		}
	}()

	return result, nil
}
