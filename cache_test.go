package utils

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ExampleExpCache() {
	cc := NewExpCache[string](context.Background(), 100*time.Millisecond)
	cc.Store("key", "val")
	cc.Load("key") // return "val"

	// data expired
	time.Sleep(200 * time.Millisecond)
	data, ok := cc.Load("key")
	fmt.Println(data)
	fmt.Println(ok)

	// Output:
	// false
}

func TestExpCache_Store(t *testing.T) {
	t.Parallel()

	Clock.SetInterval(1 * time.Millisecond)
	time.Sleep(time.Second) // wait for clock's interval to take effect

	startAt := Clock.GetUTCNow()
	ttl := 100 * time.Millisecond
	cm := NewExpCache[string](context.Background(), ttl)
	key := "key"
	val := "val"
	cm.Store(key, val)
	for {
		now := Clock.GetUTCNow()
		if gotV, ok := cm.Load(key); ok {
			require.Equal(t, val, gotV)
			require.Less(t, now.Sub(startAt), ttl)
			time.Sleep(10 * time.Millisecond)
		} else {
			require.Greater(t, now.Sub(startAt), ttl)
			break
		}
	}

	_, ok := cm.Load(key)
	require.False(t, ok)
}

// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils
// BenchmarkExpMap-8   	  141680	     10275 ns/op	      54 B/op	       6 allocs/op
// PASS
// ok  	github.com/Laisky/go-utils	1.573s
func BenchmarkExpMap(b *testing.B) {
	cm, err := NewLRUExpiredMap(context.Background(),
		10*time.Millisecond,
		func() any { return 1 },
	)
	if err != nil {
		b.Fatalf("%+v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cm.Get(RandomStringWithLength(1))
		}
	})
}

func Benchmark_NewSimpleExpCache(b *testing.B) {
	c := NewSingleItemExpCache[string](time.Millisecond)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if rand.Intn(10) < 5 {
				c.Set(RandomStringWithLength(rand.Intn(100)))
			} else {
				c.Get()
			}
		}
	})
}

func TestNewSimpleExpCache(t *testing.T) {
	t.Parallel()

	// another test may change the clock's interval.
	// default interval is 10ms, so we need to set interval bigger than 10ms.
	//
	// time.clock's test set interval to 100ms.
	fmt.Println("interval", Clock.Interval())
	Clock.SetInterval(1 * time.Millisecond)
	time.Sleep(time.Second) // wait for clock's interval to take effect

	// This test case used to have a small chance of failure
	for i := 0; i < 30; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			c := NewSingleItemExpCache[string](10 * time.Millisecond)

			_, ok := c.Get()
			require.False(t, ok)
			_, ok = c.Get()
			require.False(t, ok)
			_, ok = c.Get()
			require.False(t, ok)

			data := "yo"
			c.Set(data)
			v, ok := c.Get()
			require.True(t, ok)
			require.Equal(t, data, v)

			ret, ok := c.Get()
			require.True(t, ok)
			require.Equal(t, data, ret)

			time.Sleep(25 * time.Millisecond)
			v, ok = c.Get()
			require.False(t, ok)
			require.Equal(t, data, v)
		})
	}
}

func TestNewExpiredMap(t *testing.T) {
	ctx := context.Background()
	m, err := NewLRUExpiredMap(ctx, time.Millisecond, func() any { return 666 })
	require.NoError(t, err)

	const key = "key"
	v := m.Get(key)
	require.Equal(t, 666, v)
	v = m.Get(key)
	require.Equal(t, 666, v)
}

// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v4
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_TtlCache
// Benchmark_TtlCache/set
// Benchmark_TtlCache/set-104 	  107455	     13311 ns/op	     362 B/op	      10 allocs/op
// Benchmark_TtlCache/get
// Benchmark_TtlCache/get-104 	  740449	      1676 ns/op	      16 B/op	       1 allocs/op
// Benchmark_TtlCache/get_&_set
// Benchmark_TtlCache/get_&_set-104    57244	     20544 ns/op	     231 B/op	      10 allocs/op
func Benchmark_TtlCache(b *testing.B) {
	c := NewTtlCache[string]()
	start := time.Now().Nanosecond()

	b.Run("set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := start + i
			c.Set(strconv.Itoa(v), strconv.Itoa(v), time.Millisecond*100)
		}
	})

	b.Run("get", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Get(strconv.Itoa(start + i))
		}
	})

	b.Run("get & set", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				v := start + rand.Intn(b.N)
				c.Set(strconv.Itoa(v), strconv.Itoa(v), time.Millisecond*100)
				c.Get(strconv.Itoa(v))
			}
		})
	})
}

// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v4
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_ExpCache
// Benchmark_ExpCache/set
// Benchmark_ExpCache/set-104 	  198330	      8029 ns/op	     285 B/op	       7 allocs/op
// Benchmark_ExpCache/get
// Benchmark_ExpCache/get-104 	  912698	      1320 ns/op	      16 B/op	       1 allocs/op
// Benchmark_ExpCache/get_&_set
// Benchmark_ExpCache/get_&_set-104         	   61234	     21028 ns/op	     299 B/op	       8 allocs/op
func Benchmark_ExpCache(b *testing.B) {
	ctx := context.Background()
	c := NewExpCache[string](ctx, time.Millisecond*100)
	start := time.Now().Nanosecond()

	b.Run("set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := start + i
			c.Store(strconv.Itoa(v), strconv.Itoa(v))
		}
	})

	b.Run("get", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Load(strconv.Itoa(start + i))
		}
	})

	b.Run("get & set", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				v := start + rand.Intn(b.N)
				c.Store(strconv.Itoa(v), strconv.Itoa(v))
				c.Load(strconv.Itoa(v))
			}
		})
	})
}
