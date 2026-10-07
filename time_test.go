package utils

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// TestTimeZone verifies that a time parsed with a +10:00 offset reports a zero offset in TimeZoneUTC and a +08:00
// offset in both TimeZoneShanghai and the location loaded from "Asia/Shanghai".
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

// TestParseTs2String verifies that ParseTs2String formats several Unix timestamps as the expected RFC 3339 UTC
// strings.
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

// TestTimeCompare verifies that RFC 3339 parsing keeps fractional seconds, so times 123ms apart are not equal
// until truncated to the second, and that ParseTimeWithTruncate performs that truncation.
func TestTimeCompare(t *testing.T) {
	str1 := "2019-10-12T02:03:14Z"
	str2 := "2019-10-12T02:03:14.123Z"

	ts1, err := time.Parse(time.RFC3339, str1)
	require.NoError(t, err)
	ts2, err := time.Parse(time.RFC3339, str2)
	require.NoError(t, err)

	// ⚠️ Even though RFC3339 doesn't specifically define milliseconds,
	// if the string has a decimal point, the parsing result
	// will actually include the millisecond automatically
	require.False(t, ts1.Equal(ts2))
	require.True(t, ts2.After(ts1))

	// truncate to second
	require.True(t, ts1.Equal(ts2.Truncate(time.Second)))

	// If you want to compare times, you need to manually adjust the precision
	// after `Parse` through `Truncate`
	t.Run("truncate", func(t *testing.T) {
		ts2, err := ParseTimeWithTruncate(time.RFC3339, str2, time.Second)
		require.NoError(t, err)

		require.True(t, ts1.Equal(ts2))
	})
}

// TestParseUnix2UTC verifies that ParseUnix2UTC and ParseUnixNano2UTC convert second and nanosecond Unix
// timestamps into the expected UTC times.
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

// TestParseHex2UTC verifies that ParseHex2UTC and ParseHexNano2UTC decode hexadecimal second and nanosecond
// Unix timestamps into the expected UTC times.
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

// ExampleClock demonstrates reading the current UTC time and RFC 3339 string from the shared Clock, changing its
// refresh interval with SetInternalClock, and creating a separate clock with NewClock.
func ExampleClock() {
	// use internal clock
	// get utc now
	Clock.GetUTCNow()

	// get time string
	Clock.GetTimeInRFC3339Nano()

	// change clock refresh step
	SetInternalClock(10 * time.Millisecond)

	// create new clock, and stop it when it is no longer needed
	c := NewClock(context.Background(), 1*time.Second)
	defer c.Close()
	c.GetUTCNow()
}

// TestClock verifies the ClockT contract without depending on when the refresh loop happens to run: a clock
// whose one-hour interval has not elapsed keeps returning the time it was created with, and its string, hex,
// nanosecond-hex and date views all decode back to that cached time; a clock with a one-millisecond interval
// advances and never reports a time ahead of the real clock; SetInterval updates Interval; and a clock can be
// stopped by canceling its context, by Close, or by both.
func TestClock(t *testing.T) {
	t.Parallel()
	// The subtests are parallel and run after this function returns, so their clocks must not share a context
	// that a deferred cancel would end early; the test context lives until every subtest has finished.
	ctx := t.Context()

	t.Run("cached value is stable until the interval elapses", func(t *testing.T) {
		t.Parallel()
		beforeCreate := time.Now()
		c := NewClock(ctx, time.Hour)
		defer c.Close()
		require.Equal(t, time.Hour, c.Interval())

		ts := c.GetUTCNow()
		require.Equal(t, time.UTC, ts.Location())
		require.False(t, ts.Before(beforeCreate), "cached time predates NewClock")
		require.False(t, ts.After(time.Now()), "cached time is ahead of the real clock")

		// The loop waits out the hour before its first refresh, so every read below sees the same instant.
		for range 3 {
			require.True(t, c.GetUTCNow().Equal(ts))
			require.Equal(t, ts.Format(time.RFC3339Nano), c.GetTimeInRFC3339Nano())
		}

		hexTime, err := ParseHex2UTC(c.GetTimeInHex())
		require.NoError(t, err)
		require.True(t, hexTime.Equal(ts.Truncate(time.Second)), "hex time %s != %s", hexTime, ts)

		nanoTime, err := ParseHexNano2UTC(c.GetNanoTimeInHex())
		require.NoError(t, err)
		require.True(t, nanoTime.Equal(ts), "nano hex time %s != %s", nanoTime, ts)

		date, err := c.GetDate()
		require.NoError(t, err)
		require.True(t, date.Equal(ts.Truncate(24*time.Hour)))
	})

	t.Run("refreshing clock advances and never runs ahead", func(t *testing.T) {
		t.Parallel()
		c := NewClock(ctx, time.Millisecond)
		defer c.Close()
		ts := c.GetUTCNow()

		require.Eventually(t, func() bool {
			got := c.GetUTCNow()
			return got.After(ts) && !got.After(time.Now())
		}, 10*time.Second, time.Millisecond, "clock must refresh to a time no later than the real clock")

		c.SetInterval(100 * time.Millisecond)
		require.Equal(t, 100*time.Millisecond, c.Interval())
	})

	t.Run("stop by context cancel, Close, or both", func(t *testing.T) {
		t.Parallel()
		ctx2, cancel2 := context.WithCancel(ctx)
		c1 := NewClock(ctx2, time.Second)
		cancel2()
		c1.Close()

		c2 := NewClock(ctx, time.Second)
		c2.Close()
		c2.Close()
	})
}

// Benchmark_time measures the baseline cost of time.Now, time.Now().UTC, time.Unix, time.Unix(...).UTC, and
// time.Unix(...).UTC with an atomically loaded timestamp.
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

// BenchmarkClock measures ClockT.GetUTCNow with refresh intervals from 500ms down to 1us, using
// time.Now().UTC() as the baseline.
//
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
BenchmarkClock/clock2_time_with_10us#01-8               270614050                4.442 ns/op           0 B/op          0 allocs/op
*/
func BenchmarkClock(b *testing.B) {
	var err error
	if err = log.Shared.ChangeLevel("error"); err != nil {
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
	clock2 := NewClock(context.Background(), 500*time.Millisecond)
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

// TestSetupClock verifies that SetInternalClock accepts a 100ms interval and panics for a 1ns interval, and that
// NewClock and ClockT.SetInterval panic with "interval must greater than 1us" for sub-microsecond intervals.
func TestSetupClock(t *testing.T) {
	// The package-level Clock is shared by the whole test binary, so restore its interval afterwards.
	previous := Clock.Interval()
	t.Cleanup(func() { SetInternalClock(previous) })

	SetInternalClock(100 * time.Millisecond)
	require.Equal(t, 100*time.Millisecond, Clock.Interval())

	// case: invalid interval
	{
		ok := IsPanic(func() {
			SetInternalClock(time.Nanosecond)
		})
		require.True(t, ok)
	}

	t.Run("new clock invalid interval", func(t *testing.T) {
		require.PanicsWithValue(t, "interval must greater than 1us", func() {
			_ = NewClock(context.Background(), time.Nanosecond)
		})
	})

	t.Run("set interval invalid interval", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := NewClock(ctx, time.Millisecond)
		require.PanicsWithValue(t, "interval must greater than 1us", func() {
			c.SetInterval(time.Nanosecond)
		})
	})
}

// TestSleepWithContext verifies that SleepWithContext sleeps for the full duration with a background context and
// returns shortly after the context's 10ms timeout instead of sleeping for an hour.
func TestSleepWithContext(t *testing.T) {
	t.Run("normal sleep", func(t *testing.T) {
		startAt := time.Now()
		SleepWithContext(context.Background(), time.Millisecond*10)
		require.Greater(t, time.Since(startAt), 10*time.Millisecond)
	})

	t.Run("sleep break by context", func(t *testing.T) {
		startAt := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
		defer cancel()
		SleepWithContext(ctx, time.Hour)
		require.Less(t, time.Since(startAt), time.Second*2)
		require.Greater(t, time.Since(startAt), time.Millisecond*10)
	})
}

// TestTimeEqual verifies that TimeEqual with a one-second tolerance treats times 123ms apart as equal, even with
// different time zone offsets, and treats times 1.123s apart as not equal.
func TestTimeEqual(t *testing.T) {
	t.Parallel()

	t.Run("eq: no timezone", func(t *testing.T) {
		t.Parallel()
		str1 := "2019-10-12T02:03:14Z"
		str2 := "2019-10-12T02:03:14.123Z"

		// RFC3339     = "2006-01-02T15:04:05Z07:00"
		t1, err := time.Parse(time.RFC3339, str1)
		require.NoError(t, err)
		t2, err := time.Parse(time.RFC3339, str2)
		require.NoError(t, err)

		require.False(t, t1.Equal(t2))
		require.True(t, TimeEqual(t1, t2, time.Second))
	})

	t.Run("ne: no timezone", func(t *testing.T) {
		t.Parallel()
		str1 := "2019-10-12T02:03:14Z"
		str2 := "2019-10-12T02:03:15.123Z"

		t1, err := time.Parse(time.RFC3339, str1)
		require.NoError(t, err)
		t2, err := time.Parse(time.RFC3339, str2)
		require.NoError(t, err)

		require.False(t, t1.Equal(t2))
		require.False(t, TimeEqual(t1, t2, time.Second))
	})

	t.Run("eq: in different timezone", func(t *testing.T) {
		t.Parallel()
		str1 := "2019-10-12T02:03:14+07:00"
		str2 := "2019-10-12T03:03:14.123+08:00"

		t1, err := time.Parse(time.RFC3339, str1)
		require.NoError(t, err)
		t2, err := time.Parse(time.RFC3339, str2)
		require.NoError(t, err)

		require.False(t, t1.Equal(t2))
		require.True(t, TimeEqual(t1, t2, time.Second))
	})
}
