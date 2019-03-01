package utils_test

import (
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

func ExampleClock() {
	// get utc now
	utils.Clock.GetUTCNow()

	// get time string
	utils.Clock.GetTimeInRFC3339Nano()

	// change clock refresh step
	utils.SetupClock(10 * time.Millisecond)
}

func TestClock(t *testing.T) {
	utils.SetupClock(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond) // first refresh

	// test ts
	ts := utils.Clock.GetUTCNow()
	if utils.Clock.GetUTCNow().Sub(ts) > 1*time.Nanosecond {
		t.Fatalf("should got same ts")
	}
	if utils.Clock.GetUTCNow().Sub(ts) > 1*time.Nanosecond {
		t.Fatalf("should got same ts")
	}
	time.Sleep(50 * time.Millisecond)
	if utils.Clock.GetUTCNow().Sub(ts) > 1*time.Nanosecond {
		t.Fatalf("should got same ts")
	}
	if utils.Clock.GetUTCNow().Sub(ts) > 1*time.Nanosecond {
		t.Fatalf("should got same ts")
	}
	time.Sleep(50 * time.Millisecond)
	if utils.Clock.GetUTCNow().Sub(ts) < 100*time.Millisecond {
		t.Fatalf("should not got same ts")
	}

	// test ts string
	timeStr := utils.Clock.GetTimeInRFC3339Nano()
	if utils.Clock.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	if utils.Clock.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	time.Sleep(50 * time.Millisecond)
	if utils.Clock.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	if utils.Clock.GetTimeInRFC3339Nano() != timeStr {
		t.Fatalf("should got same time string")
	}
	time.Sleep(50 * time.Millisecond)
	if utils.Clock.GetTimeInRFC3339Nano() == timeStr {
		t.Fatalf("should not got same time string")
	}
}

func BenchmarkClock(b *testing.B) {
	b.Run("normal time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			time.Now().UTC()
		}
	})

	utils.SetupClock(500 * time.Millisecond)
	b.Run("clock time with 500ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Clock.GetUTCNow()
		}
	})

	utils.SetupClock(100 * time.Millisecond)
	b.Run("clock time with 100ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Clock.GetUTCNow()
		}
	})

	utils.SetupClock(1 * time.Millisecond)
	b.Run("clock time with 1ms", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Clock.GetUTCNow()
		}
	})
}
