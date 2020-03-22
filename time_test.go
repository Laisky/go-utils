package utils

import (
	"context"
	"testing"
	"time"
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
	SetupClock(10 * time.Millisecond)

	// create new clock
	c := NewClock(context.Background(), 1*time.Second)
	c.GetUTCNow()
}

func TestClock2(t *testing.T) {
	var (
		c   = NewClock2(context.Background(), 100*time.Millisecond)
		ts  = c.GetUTCNow()
		err error
	)
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

}

// func TestLoop(t *testing.T) {
// 	for {
// 		time.Sleep(1 * time.Millisecond)
// 	}
// }

/*
BenchmarkClock/normal_time-4         	10745672	       105 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_500ms-4         	208522263	         5.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_100ms-4         	217223018	         5.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_10ms-4          	206468820	         5.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_1ms-4           	212732216	         5.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_500us-4         	206800707	         5.56 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_100us-4         	214629580	         5.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_10us-4          	196311190	         6.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkClock/clock2_time_with_10us#01-4       	167978643	         6.56 ns/op	       0 B/op	       0 allocs/op

*/
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

	// clock 2
	clock2 := NewClock2(context.Background(), 500*time.Millisecond)
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
	clock2.SetupInterval(500 * time.Microsecond)
	b.Run("clock2 time with 500us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(100 * time.Microsecond)
	b.Run("clock2 time with 100us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(10 * time.Microsecond)
	b.Run("clock2 time with 10us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
	clock2.SetupInterval(1 * time.Microsecond)
	b.Run("clock2 time with 10us", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			clock2.GetUTCNow()
		}
	})
}
