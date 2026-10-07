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

// ExampleExpCache demonstrates storing a value in an ExpCache with a 100ms ttl and shows that Load returns the
// zero value and false once the entry has expired.
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

// cacheWallNow returns the current wall-clock time in UTC without a monotonic reading. The caches stamp and check
// expirations with time.Now().UTC(), so timestamps taken with this helper live in the same clock domain and can
// bracket a cache decision exactly. It takes no parameters and returns the current time.
func cacheWallNow() time.Time {
	return time.Now().UTC()
}

// requireHitThenExpiry checks the expiring-cache contract against measured time instead of assumed sleep lengths:
// a value read back before ttl has elapsed since it was stored is returned, and once ttl has elapsed it is reported
// as missing. It takes the test handle, the cache ttl, the stored value, and store and load closures over the cache
// under test; it returns nothing and fails the test on a contract violation. Because a loaded host may stall the
// test for longer than ttl between store and load, an attempt whose read-back finished ttl or more after the store
// began proves nothing and is retried with a fresh store until a deadline passes.
func requireHitThenExpiry[T any](t *testing.T, ttl time.Duration, want T, store func(T), load func() (T, bool)) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		beforeStore := cacheWallNow()
		store(want)
		afterStore := cacheWallNow()
		got, ok := load()
		if cacheWallNow().Sub(beforeStore) >= ttl {
			require.True(t, time.Now().Before(deadline),
				"never managed to read a value back within its %s ttl", ttl)
			continue
		}

		// The value expires no earlier than beforeStore+ttl, and load ended before that.
		require.True(t, ok, "value read back within its ttl must be returned")
		require.Equal(t, want, got)

		// The value expires no later than afterStore+ttl; wait until that moment has certainly passed.
		expiredBy := afterStore.Add(ttl)
		for now := cacheWallNow(); !now.After(expiredBy); now = cacheWallNow() {
			time.Sleep(expiredBy.Sub(now) + time.Millisecond)
		}
		_, ok = load()
		require.False(t, ok, "value must be reported missing once its ttl has elapsed")
		return
	}
}

// TestExpCache_Store verifies that a value stored in an ExpCache with a 100ms ttl is returned by Load only while
// it is younger than the ttl, that Load reports a miss only after the ttl has elapsed, and that the miss persists.
// Every assertion compares against timestamps that bracket the cache operations, so it holds on a loaded host.
func TestExpCache_Store(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const ttl = 100 * time.Millisecond
	cm := NewExpCache[string](ctx, ttl)
	key := "key"
	val := "val"

	beforeStore := cacheWallNow()
	cm.Store(key, val)
	afterStore := cacheWallNow()
	for {
		beforeLoad := cacheWallNow()
		gotV, ok := cm.Load(key)
		afterLoad := cacheWallNow()
		if ok {
			require.Equal(t, val, gotV)
			// A hit means Load ran before the entry expired, which happens no later than afterStore+ttl.
			require.Less(t, beforeLoad.Sub(afterStore), ttl, "Load returned a value older than its ttl")
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// A miss means Load ran after the entry expired, which happens no earlier than beforeStore+ttl.
		require.GreaterOrEqual(t, afterLoad.Sub(beforeStore), ttl, "Load dropped a value before its ttl")
		break
	}

	_, ok := cm.Load(key)
	require.False(t, ok)
}

// BenchmarkExpMap measures parallel LRUExpiredMap.Get calls on random one-character keys with a 10ms ttl.
//
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

// Benchmark_NewSimpleExpCache measures concurrent access to a SingleItemExpCache with a 1ms ttl, where each
// iteration randomly either sets a random string of up to 99 characters or reads the cached value.
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

// TestNewSimpleExpCache verifies, across 30 parallel subtests, that a new SingleItemExpCache with a 10ms ttl
// reports a miss before any Set, returns the stored value when it is read back within the ttl, and reports a miss
// while still returning the last value once the ttl has elapsed.
func TestNewSimpleExpCache(t *testing.T) {
	t.Parallel()

	for i := 0; i < 30; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			const ttl = 10 * time.Millisecond
			c := NewSingleItemExpCache[string](ttl)

			_, ok := c.Get()
			require.False(t, ok)
			_, ok = c.Get()
			require.False(t, ok)
			_, ok = c.Get()
			require.False(t, ok)

			data := "yo"
			requireHitThenExpiry(t, ttl, data, c.Set, c.Get)

			v, ok := c.Get()
			require.False(t, ok)
			require.Equal(t, data, v)
		})
	}
}

// TestNewExpiredMap verifies that LRUExpiredMap.Get creates a missing key with the constructor's value (666) and
// returns the same value on a repeated Get of that key.
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

// TestNewLruCache verifies that a cache created by NewLruCache with capacity 100 and a 100ms ttl returns each
// value right after it is set and still returns earlier keys after new keys are added.
func TestNewLruCache(t *testing.T) {
	t.Parallel()

	c := NewLruCache[string, string](100, time.Millisecond*100)
	c.Set("key", "val")
	v, ok := c.Get("key")
	require.True(t, ok)
	require.Equal(t, "val", v)

	c.Set("key2", "val2")
	v, ok = c.Get("key2")
	require.True(t, ok)
	require.Equal(t, "val2", v)

	v, ok = c.Get("key")
	require.True(t, ok)
	require.Equal(t, "val", v)

	c.Set("key3", "val3")
	v, ok = c.Get("key3")
	require.True(t, ok)
	require.Equal(t, "val3", v)

	c.Set("key4", "val4")
	v, ok = c.Get("key4")
	require.True(t, ok)
	require.Equal(t, "val4", v)
}

// Benchmark_TtlCache measures TtlCache with a 100ms ttl in three sub-benchmarks: sequential Set, sequential Get,
// and parallel Set followed by Get on random keys.
//
// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v6
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

// Benchmark_ExpCache measures ExpCache with a 100ms ttl in three sub-benchmarks: sequential Store, sequential
// Load, and parallel Store followed by Load on random keys.
//
// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v6
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

// Benchmark_Sieve measures the sieve cache returned by NewLruCache (capacity 100000, 100ms ttl) in three
// sub-benchmarks: sequential Set, sequential Get, and parallel Set followed by Get on random keys.
//
// pkg: github.com/Laisky/go-utils/v6
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// Benchmark_Sieve
// Benchmark_Sieve/set
// Benchmark_Sieve/set-104		144274	      9077 ns/op	     186 B/op	       4 allocs/op
// Benchmark_Sieve/get
// Benchmark_Sieve/get-104		587437	      1795 ns/op	      16 B/op	       1 allocs/op
// Benchmark_Sieve/get_&_set
// Benchmark_Sieve/get_&_set-104		67621	     20670 ns/op	     179 B/op	       5 allocs/op
// PASS
func Benchmark_Sieve(b *testing.B) {
	c := NewLruCache[string, string](100000, time.Millisecond*100)
	start := time.Now().Nanosecond()

	b.Run("set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := start + i
			c.Set(strconv.Itoa(v), strconv.Itoa(v))
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
				c.Set(strconv.Itoa(v), strconv.Itoa(v))
				c.Get(strconv.Itoa(v))
			}
		})
	})
}

// TestCacheTimingPrecision verifies that SingleItemExpCache and ExpCache with a 5ms ttl return a value read back
// within the ttl and report a miss once the ttl has elapsed, measured against timestamps that bracket each call.
func TestCacheTimingPrecision(t *testing.T) {
	t.Parallel()
	const ttl = 5 * time.Millisecond

	t.Run("SingleItemExpCache", func(t *testing.T) {
		t.Parallel()

		cache := NewSingleItemExpCache[string](ttl)
		requireHitThenExpiry(t, ttl, "test-value", cache.Set, cache.Get)
	})

	t.Run("ExpCache", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cache := NewExpCache[string](ctx, ttl)
		requireHitThenExpiry(t, ttl, "test-value",
			func(v string) { cache.Store("test-key", v) },
			func() (string, bool) { return cache.Load("test-key") })
	})
}
