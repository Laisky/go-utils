package utils

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	gset "github.com/deckarep/golang-set/v2"
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

func FuzzHumanReadableByteCount(f *testing.F) {
	type args struct {
		bytes int64
		si    bool
	}
	for _, test := range []struct {
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
	} {
		f.Add(test.args.bytes, test.args.si)
	}

	f.Fuzz(func(t *testing.T, bytes int64, si bool) {
		recv := HumanReadableByteCount(bytes, si)
		require.Contains(t, []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}, string(recv[len(recv)-1]))
		require.Greater(t, len(recv), 0)
	})

}

// func TestHumanReadableByteCount(t *testing.T) {
// 	type args struct {
// 		bytes int64
// 		si    bool
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		wantRet string
// 	}{
// 		{"0", args{1, true}, "1B"},
// 		{"1", args{12, true}, "12B"},
// 		{"2", args{0, true}, "0B"},
// 		{"3", args{1024, true}, "1KB"},
// 		{"4", args{1025, true}, "1KB"},
// 		{"5", args{1004, false}, "1KB"},
// 		{"6", args{1005, false}, "1.01KB"},
// 		{"7", args{1006, false}, "1.01KB"},
// 		{"8", args{1000000, false}, "1MB"},
// 		{"9", args{1005000, false}, "1.01MB"},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if gotRet := HumanReadableByteCount(tt.args.bytes, tt.args.si); gotRet != tt.wantRet {
// 				t.Errorf("HumanReadableByteCount() = %v, want %v", gotRet, tt.wantRet)
// 			}
// 		})
// 	}
// }

func FuzzMinInt(f *testing.F) {
	f.Add(1, 2)
	f.Add(-1, 2)
	f.Add(33, -2)

	f.Fuzz(func(t *testing.T, arg1, arg2 int) {
		rev1 := Min(arg1, arg2)
		rev2 := Min(arg2, arg1)
		require.Equal(t, rev1, rev2)
		require.LessOrEqual(t, rev1, arg1)
		require.LessOrEqual(t, rev1, arg2)
	})
}

func FuzzMinStr(f *testing.F) {
	f.Add("a", "b")
	f.Add("4324a", "br4r")
	f.Add("~", "&!#@*()")

	f.Fuzz(func(t *testing.T, arg1, arg2 string) {
		rev1 := Min(arg1, arg2)
		rev2 := Min(arg2, arg1)
		require.Equal(t, rev1, rev2)
		require.LessOrEqual(t, rev1, arg1)
		require.LessOrEqual(t, rev1, arg2)
	})
}

func TestMin(t *testing.T) {
	require.Equal(t, Min(-1, 2, 3), -1)
	require.Equal(t, Min(1, -2, 3), -2)
	require.Equal(t, Min(4, 2, 3), 2)
	require.Equal(t, Min(4.2, 2.0, 3.1), 2.0)
	require.Equal(t, Min("a", "b", "c"), "a")
	require.Panics(t, func() { Min[int]() })
}

func TestMax(t *testing.T) {
	require.Equal(t, Max(-1, 2, 3), 3)
	require.Equal(t, Max(1, -2, 3), 3)
	require.Equal(t, Max(4, 2, 3), 4)
	require.Equal(t, Max(4.2, 2.0, 3.1), 4.2)
	require.Equal(t, Max("a", "b", "c"), "c")
	require.Panics(t, func() { Max[int]() })
}

func TestIntersectSortedChans(t *testing.T) {
	nChan := 5

	chans := make([]chan int, nChan)
	sets := make([]gset.Set[int], nChan)
	arrs := make([][]int, nChan)
	for i := 0; i < nChan; i++ {
		chans[i] = make(chan int)
		sets[i] = gset.NewSet[int]()
		go func(i int) {
			last := 0
			randor := rand.New(rand.NewSource(time.Now().UnixNano()))
			for j := 0; j < 1000; j++ {
				last += randor.Intn(2)
				sets[i].Add(last)
				arrs[i] = append(arrs[i], last)
				chans[i] <- last
			}

			close(chans[i])
		}(i)
	}

	got := gset.NewSet[int]()
	resultChan, err := IntersectSortedChans(chans...)
	require.NoError(t, err)
	for v := range resultChan {
		got.Add(v)
	}

	EmptyAllChans(chans...)

	expect := sets[0]
	t.Log("arr<0>:", arrs[0])
	for i := 1; i < nChan; i++ {
		t.Logf("arr<%d>:%v", i, arrs[i])
		expect = expect.Intersect(sets[i])
	}

	t.Log("expect:", expect.ToSlice())
	t.Log("got:", got.ToSlice())
	require.NotEqual(t, len(got.ToSlice()), 0)
	require.True(t, got.Equal(expect))
	// t.Error()
}
