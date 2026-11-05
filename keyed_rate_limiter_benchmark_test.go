package utils

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkKeyedRateLimiter measures successful and refused hot paths, and
// admissions that need one full key evicted from a bounded table. Rejected
// cases use a fixed time so they remain rejected regardless of test duration.
func BenchmarkKeyedRateLimiter(b *testing.B) {
	keys := make([]string, 1025)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
	}
	for _, tc := range []struct {
		name               string
		count              int
		accepted, parallel bool
	}{
		{"accept/one/serial", 1, true, false}, {"accept/one/parallel", 1, true, true},
		{"accept/1024/serial", 1024, true, false}, {"accept/1024/parallel", 1024, true, true},
		{"reject-fixed/one/serial", 1, false, false}, {"reject-fixed/one/parallel", 1, false, true},
		{"reject-fixed/1024/serial", 1024, false, false}, {"reject-fixed/1024/parallel", 1024, false, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			cfg := KeyedRateLimiterConfig{TotalPerSecond: 1_000_000_000, TotalBurst: 1_000_000_000, KeyPerSecond: 1_000_000_000, KeyBurst: 1_000_000_000, MaxKeys: tc.count}
			if !tc.accepted {
				cfg.KeyPerSecond = 1
				cfg.KeyBurst = 1
			}
			l, err := NewKeyedRateLimiter[string](cfg)
			if err != nil {
				b.Fatal(err)
			}
			allow := l.Allow
			if !tc.accepted {
				now := time.Unix(100, 0)
				allow = func(key string) bool { return l.allowAt(key, now) }
			}
			for _, k := range keys[:tc.count] {
				if !allow(k) {
					b.Fatal("warmup refused")
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			if tc.parallel {
				b.RunParallel(func(p *testing.PB) {
					i := 0
					for p.Next() {
						ok := allow(keys[i%tc.count])
						if ok != tc.accepted {
							panic("unexpected admission")
						}
						i++
					}
				})
			} else {
				for i := 0; i < b.N; i++ {
					if allow(keys[i%tc.count]) != tc.accepted {
						b.Fatal("unexpected admission")
					}
				}
			}
		})
	}
	b.Run("evict/1024/serial", func(b *testing.B) {
		cfg := KeyedRateLimiterConfig{TotalPerSecond: 1_000_000_000, TotalBurst: 1_000_000_000, KeyPerSecond: 1_000_000_000, KeyBurst: 1, MaxKeys: 1024}
		l, err := NewKeyedRateLimiter[string](cfg)
		if err != nil {
			b.Fatal(err)
		}
		for _, k := range keys[:1024] {
			if !l.Allow(k) {
				b.Fatal("warmup refused")
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !l.Allow(keys[i%len(keys)]) {
				b.Fatal("refused")
			}
		}
	})
}
