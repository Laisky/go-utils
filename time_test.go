package utils

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeZone(t *testing.T) {
	ts := "2021-10-24T20:00:00+10:00"
	tt, err := time.Parse(time.RFC3339, ts)
	require.NoError(t, err)

	_, offset := tt.Zone()
	require.Equal(t, 10*3600, offset)

	tt = tt.In(TimeZoneUTC)
	_, offset = tt.Zone()
	require.Equal(t, 0, offset)

	tt = tt.In(TimeZoneShanghai)
	_, offset = tt.Zone()
	require.Equal(t, 8*3600, offset)

	tz, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	tt = tt.In(tz)
	_, offset = tt.Zone()
	require.Equal(t, 8*3600, offset)
}

func TestParseTs2String(t *testing.T) {
	var (
		got    string
		layout = time.RFC3339
	)

	cases := map[int64]string{
		1:         "1970-01-01T00:00:01Z",
		100000:    "1970-01-02T03:46:40Z",
		100000000: "1973-03-03T09:46:40Z",
	}
	for ts, v := range cases {
		if got = ParseTs2String(ts, layout); got != v {
			t.Errorf("expect %v, got %v", v, got)
		}
	}
}

func TestParseUnix2UTC(t *testing.T) {
	ut := int64(1570845794)
	ts := ParseUnix2UTC(ut).Format(time.RFC3339)
	if ts != "2019-10-12T02:03:14Z" {
		t.Fatalf("got %v", ts)
	}

	utnano := int64(1570848785196500001)
	ts = ParseUnixNano2UTC(utnano).Format(time.RFC3339Nano)
	if ts != "2019-10-12T02:53:05.196500001Z" {
		t.Fatalf("got %v", ts)
	}
}

func TestParseHex2UTC(t *testing.T) {
	hex := "5da140b4"
	ts, err := ParseHex2UTC(hex)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if ts.Format(time.RFC3339) != "2019-10-12T02:55:48Z" {
		t.Fatalf("got %v", ts.Format(time.RFC3339))
	}

	hexnano := "15ccc6cbb2f54a48"
	ts, err = ParseHexNano2UTC(hexnano)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if ts.Format(time.RFC3339Nano) != "2019-10-12T02:55:48.228541Z" {
		t.Fatalf("got %v", ts.Format(time.RFC3339Nano))
	}
}

// func TestTimeParse(t *testing.T) {
// 	s := "2018-11-12 03:41:39,735"
// 	layout := "2006-01-02 15:04:05,000"
// 	ts, err := time.Parse(layout, s)
// 	if err != nil {
// 		t.Fatalf("got error: %+v", err)
// 	}
// 	t.Logf("%+v", ts)
// }

// func TestTimeFormat(t *testing.T) {
// 	ts := time.Now()
// 	layout := "2006-01-02 15:04:05.000"
// 	t.Errorf("%+v - %+v", ts, ts.Format(layout))
// 	time.Sleep(20 * time.Millisecond)
// 	t.Errorf("%+v - %+v", ts, ts.Format(layout))
// 	time.Sleep(20 * time.Millisecond)
// 	t.Errorf("%+v - %+v", ts, ts.Format(layout))
// 	time.Sleep(20 * time.Millisecond)
// 	t.Errorf("%+v - %+v", ts, ts.Format(layout))
// 	time.Sleep(20 * time.Millisecond)
// 	t.Errorf("%+v - %+v", ts, ts.Format("2006-01-02 15:04:05.999"))
// }

func ExampleClock() {
	// use internal clock
	// get utc now
	Clock.GetUTCNow()

	// get time string
	Clock.GetTimeInRFC3339Nano()

	// change clock refresh step
	SetInternalClock(10 * time.Millisecond)

	// create new clock
	c := NewClock(context.Background(), 1*time.Second)
	c.GetUTCNow()
}

func TestClock2(t *testing.T) {
	ctx := context.Background()
	c := NewClock2(ctx, 100*time.Millisecond)
	ts := c.GetUTCNow()
	var err error
	t.Logf("ts: %v", ts.Format(time.RFC3339Nano))

	c.SetInterval(100 * time.Millisecond)

	// test ts
	time.Sleep(10 * time.Millisecond) // first refresh
	if c.GetUTCNow().After(ts) {
		t.Fatalf("should got same ts")
	}
	if c.GetUTCNow().After(ts) {
		t.Fatalf("should got same ts")
	}
	time.Sleep(50 * time.Millisecond)
	if c.GetUTCNow().After(ts) {
		t.Fatalf("should got same ts")
	}
	if c.GetUTCNow().After(ts) {
		t.Fatalf("should got same ts")
	}
	time.Sleep(50 * time.Millisecond)
	if c.GetUTCNow().Sub(ts) < 50*time.Millisecond {
		t.Fatalf("should not got same ts, got: %+v", c.GetUTCNow().Format(time.RFC3339Nano))
	}

	// test ts string
	timeStr := c.GetTimeInRFC3339Nano()
	if c.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	if c.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	time.Sleep(50 * time.Millisecond)
	if c.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	if c.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	time.Sleep(50 * time.Millisecond)
	if c.GetTimeInRFC3339Nano() == timeStr {
		t.Fatalf("should not got same time string")
	}

	// test hex
	timeStr = c.GetTimeInHex()
	if ts, err = ParseHex2UTC(timeStr); err != nil {
		t.Fatalf("try to parse timeStr got error: %+v", err)
	}
	if ts.Format(time.RFC3339) != c.GetUTCNow().Format(time.RFC3339) {
		t.Errorf("ts: %v", ts.Format(time.RFC3339))
		t.Errorf("c.get: %v", c.GetUTCNow().Format(time.RFC3339))
		t.Fatalf("hex time must equal to time")
	}

	timeStr = c.GetNanoTimeInHex()
	if ts, err = ParseHexNano2UTC(timeStr); err != nil {
		t.Fatalf("try to parse timeStr got error: %+v", err)
	}
	if ts.Format(time.RFC3339Nano) != c.GetUTCNow().Format(time.RFC3339Nano) {
		t.Errorf("ts: %v", ts.Format(time.RFC3339Nano))
		t.Errorf("c.get: %v", c.GetTimeInRFC3339Nano())
		t.Fatalf("hex time must equal to time")
	}

	// test date
	{
		_, err := c.GetDate()
		require.NoError(t, err)
	}

	// case: close clock
	{
		ctx2, cancel := context.WithCancel(ctx)
		_ = NewClock(ctx2, time.Second)
		cancel()

		ctx2, cancel = context.WithCancel(ctx)
		defer cancel()
		c2 := NewClock(ctx2, time.Second)
		c2.Close()
	}
}

func Benchmark_time(b *testing.B) {
	b.Run("normal time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Now()
		}
	})

	b.Run("normal time with UTC", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Now().UTC()
		}
	})

	b.Run("parse unix", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Unix(1623892878, 0)
		}
	})

	b.Run("parse unix with utc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Unix(1623892878, 0).UTC()
		}
	})

	var n int64 = 1623892878
	b.Run("parse unix with utc & load", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Unix(atomic.LoadInt64(&n), 0).UTC()
		}
	})

}

// func TestLoop(t *testing.T) {
// 	for {
// 		time.Sleep(1 * time.Millisecond)
// 	}
// }

/*
goos: linux
goarch: amd64
pkg: github.com/Laisky/go-utils
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz

BenchmarkClock/normal_time-8            26779118                42.67 ns/op            0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_500ms-8                 294967054                4.130 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_100ms-8                 294066337                4.153 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_10ms-8                  295792012                4.044 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_1ms-8                   284931848                4.160 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_500us-8                 293249996                4.167 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_100us-8                 291018960                4.230 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_10us-8                  294948268                4.302 ns/op           0 B/op          0 allocs/op
BenchmarkClock/clock2_time_with_10us#01-8               270614050                4.442 ns/op           0 B/op          0 allocs/op*/
func BenchmarkClock(b *testing.B) {
	var err error
	if err = Logger.ChangeLevel("error"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	// clock 1
	b.Run("normal time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Now().UTC()
		}
	})

	// b.Run("demo", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		time.Unix(8742374732483, 0).UTC()
	// 	}
	// })

	// clock 2
	clock2 := NewClock2(context.Background(), 500*time.Millisecond)
	b.Run("clock2 time with 500ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(100 * time.Millisecond)
	b.Run("clock2 time with 100ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(10 * time.Millisecond)
	b.Run("clock2 time with 10ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(1 * time.Millisecond)
	b.Run("clock2 time with 1ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(500 * time.Microsecond)
	b.Run("clock2 time with 500us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(100 * time.Microsecond)
	b.Run("clock2 time with 100us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(10 * time.Microsecond)
	b.Run("clock2 time with 10us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetInterval(1 * time.Microsecond)
	b.Run("clock2 time with 10us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
}

func TestSetupClock(t *testing.T) {
	SetInternalClock(100 * time.Millisecond)

	// case: invalid interval
	{
		ok := IsPanic(func() {
			SetInternalClock(time.Nanosecond)
		})
		require.True(t, ok)
	}
}
