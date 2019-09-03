package utils_test

import (
	"context"
	"testing"
	"time"

	utils "github.com/Laisky/go-utils"
)

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
		if got = utils.ParseTs2String(ts, layout); got != v {
			t.Errorf("expect %v, got %v", v, got)
		}
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
	// get utc now
	utils.Clock.GetUTCNow()

	// get time string
	utils.Clock.GetTimeInRFC3339Nano()

	// change clock refresh step
	utils.SetupClock(10 * time.Millisecond)
}

func TestClock(t *testing.T) {
	c := utils.NewClock(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond) // first refresh

	// test ts
	ts := c.GetUTCNow()
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
		t.Fatalf("should not got same ts")
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
}

func TestClock2(t *testing.T) {
	c := utils.NewClock2(context.Background(), 100*time.Millisecond)
	ts := c.GetUTCNow()
	t.Logf("ts: %v", ts.Format(time.RFC3339Nano))

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
}

// func TestLoop(t *testing.T) {
// 	for {
// 		time.Sleep(1 * time.Millisecond)
// 	}
// }

/*
BenchmarkClock/normal_time-4         	20000000	       109 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock_time_with_500ms-4         	100000000	        20.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock_time_with_100ms-4         	100000000	        20.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock_time_with_10ms-4          	100000000	        20.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock_time_with_1ms-4           	100000000	        22.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_500ms-4        	200000000	         6.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_100ms-4        	200000000	         5.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_10ms-4         	200000000	         6.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_1ms-4          	200000000	         6.15 ns/op	       0 B/op	       0 allocs/op
*/
func BenchmarkClock(b *testing.B) {
	utils.SetupLogger("error")
	// clock 1
	b.Run("normal time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Now().UTC()
		}
	})
	clock := utils.NewClock(500 * time.Millisecond)
	b.Run("clock time with 500ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock.GetUTCNow()
		}
	})
	clock.SetupInterval(100 * time.Millisecond)
	b.Run("clock time with 100ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock.GetUTCNow()
		}
	})
	clock.SetupInterval(10 * time.Millisecond)
	b.Run("clock time with 10ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock.GetUTCNow()
		}
	})
	clock.SetupInterval(1 * time.Millisecond)
	b.Run("clock time with 1ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock.GetUTCNow()
		}
	})

	// clock 2
	clock2 := utils.NewClock2(context.Background(), 500*time.Millisecond)
	b.Run("clock2 time with 500ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(100 * time.Millisecond)
	b.Run("clock2 time with 100ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(10 * time.Millisecond)
	b.Run("clock2 time with 10ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(1 * time.Millisecond)
	b.Run("clock2 time with 1ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
}
