package utils

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAbsInt64(t *testing.T) {
	// int64: -9223372036854775808 ~ 9223372036854775807
	// Abs(math.MinInt64) == math.MinInt64
	v := int(math.Abs(float64(math.MinInt64)))
	require.Equal(t, v, math.MinInt64)

	type args struct {
		v int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{"0", args{int64(math.MaxInt64)}, int64(math.MaxInt64)},
		{"1", args{int64(math.MinInt64)}, int64(math.MaxInt64)},
		{"1", args{int64(math.MinInt64 + 1)}, int64(math.MaxInt64)},
		{"1", args{int64(math.MinInt64 + 2)}, int64(math.MaxInt64 - 1)},
		{"2", args{int64(0)}, int64(0)},
		{"3", args{int64(-1)}, int64(1)},
		{"4", args{int64(1)}, int64(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AbsInt64(tt.args.v); got != tt.want {
				t.Errorf("AbsInt64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAbsInt32(t *testing.T) {
	type args struct {
		v int32
	}
	tests := []struct {
		name string
		args args
		want int32
	}{
		{"0", args{int32(math.MaxInt32)}, int32(math.MaxInt32)},
		{"1", args{int32(math.MinInt32)}, int32(math.MaxInt32)},
		{"1", args{int32(math.MinInt32 + 1)}, int32(math.MaxInt32)},
		{"1", args{int32(math.MinInt32 + 2)}, int32(math.MaxInt32 - 1)},
		{"2", args{int32(0)}, int32(0)},
		{"3", args{int32(-1)}, int32(1)},
		{"4", args{int32(1)}, int32(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AbsInt32(tt.args.v); got != tt.want {
				t.Errorf("AbsInt32() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRound(t *testing.T) {
	{
		r32 := float32(1.005) * 100                  // 100.5
		r64 := float64(1.005) * 100                  // 100.499
		r64v2 := 1.005 * 100                         // 100.5
		r64v3 := 1.005 * float64(100)                // 100.499
		r64v4 := float64(1.005) * 100.0              // 100.499
		r64v5 := math.Round(float64(1.005) * 100.0)  // 100
		r64v6 := fmt.Sprintf("%.2f", float64(1.005)) // "1.00"
		r2 := Round(1.005, 2)
		t.Log(r32, r64, r64v2, r64v3, r64v4, r64v5, r64v6)
		t.Log(r2)
	}

	{
		const m float64 = 100.0
		r := 1.005 * m
		t.Log(r) // 100.499
	}

	{
		var n = 1.005
		const m = 1.005
		const m2 = float64(1.005)
		rn := n * n    // 1.01002499
		rm := m * m    // 1.010025
		rm2 := m2 * m2 // 1.01002499
		t.Log(rn, rm, rm2)
	}
}

func TestHumanReadableByteCount(t *testing.T) {
	type args struct {
		bytes int64
		si    bool
	}
	tests := []struct {
		name    string
		args    args
		wantRet string
	}{
		{"0", args{1, true}, "1B"},
		{"1", args{12, true}, "12B"},
		{"2", args{0, true}, "0B"},
		{"3", args{1024, true}, "1KB"},
		{"4", args{1025, true}, "1KB"},
		{"5", args{1004, false}, "1KB"},
		{"6", args{1005, false}, "1.01KB"},
		{"7", args{1006, false}, "1.01KB"},
		{"8", args{1000000, false}, "1MB"},
		{"9", args{1005000, false}, "1.01MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotRet := HumanReadableByteCount(tt.args.bytes, tt.args.si); gotRet != tt.wantRet {
				t.Errorf("HumanReadableByteCount() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}
