package counter

import (
	"testing"

	"github.com/Laisky/go-utils/v6/log"
)

// BenchmarkCounter/count_1-8         	 1369930	       920 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/get_speed-8       	  620430	      2278 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_1_parallel_4-8         	  212336	      5285 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_5-8                    	18502207	        64.3 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_500-8                  	18213850	        64.1 ns/op	       0 B/op	       0 allocs/op
// BenchmarkCounter/count_500_parallel_4-8       	  239703	      5315 ns/op	       0 B/op	       0 allocs/op
func BenchmarkCounter(b *testing.B) {
	counter := NewCounter()

	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("get speed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.GetSpeed()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.CountN(500)
			}
		})
	})
}

func BenchmarkRotateCounter(b *testing.B) {
	counter, err := NewRotateCounter(1000000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.CountN(500)
			}
		})
	})
}

/*
BenchmarkAllCounter

✗ go test -run=All -bench=AllCo -benchtime=5s -benchmem
goos: darwin
goarch: amd64
pkg: github.com/Laisky/go-utils
BenchmarkAllCounter/atomicCounter_count_1-4             833836315                6.99 ns/op            0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_count_1-4             26496855               219 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_count_1-4              195491630               30.2 ns/op             1 B/op          0 allocs/op
BenchmarkAllCounter/atomicCounter_count_500-4           821179578                7.09 ns/op            0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_count_500-4              54483            108021 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_count_500-4              372174             15063 ns/op             960 B/op          5 allocs/op
BenchmarkAllCounter/atomicCounter_parallel-4_count_1-4          68061858               108 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_parallel-4_count_1-4           5469538              1221 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_parallel-4_count_1-4           30513360               211 ns/op               7 B/op          0 allocs/op
BenchmarkAllCounter/atomicCounter_parallel-4_count_500-4        63054807               107 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/rotateCounter_parallel-4_count_500-4            9793            613852 ns/op               0 B/op          0 allocs/op
BenchmarkAllCounter/childCounter_parallel-4_count_500-4            60672            102970 ns/op            3840 B/op         20 allocs/op
PASS
ok      github.com/Laisky/go-utils      82.997s
*/
func BenchmarkAllCounter(b *testing.B) {
	b.ReportAllocs()
	var err error
	prevLevel := log.Shared.Level()
	if err = log.Shared.ChangeLevel("info"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	b.Cleanup(func() {
		if restoreErr := log.Shared.ChangeLevel(prevLevel); restoreErr != nil {
			b.Errorf("restore log level %q: %+v", prevLevel, restoreErr)
		}
	})
	atomicCounter := NewCounter()
	rotateCounter, err := NewRotateCounter(100000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	parallelCounter, err := NewParallelCounter(100, 100000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	childCounter := parallelCounter.GetChild()

	// count 1
	b.Run("atomicCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			atomicCounter.Count()
		}
	})
	b.Run("rotateCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rotateCounter.Count()
		}
	})
	b.Run("childCounter count 1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			childCounter.Count()
		}
	})

	// count 500
	b.Run("atomicCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			atomicCounter.CountN(500)
		}
	})
	b.Run("rotateCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rotateCounter.CountN(500)
		}
	})
	b.Run("childCounter count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			childCounter.CountN(500)
		}
	})

	// parallel count 1
	b.Run("atomicCounter parallel-4 count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.Count()
			}
		})
	})
	b.Run("rotateCounter parallel-4 count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.Count()
			}
		})
	})
	b.Run("childCounter parallel-4 count 1", func(b *testing.B) {
		cc1 := parallelCounter.GetChild()
		cc2 := parallelCounter.GetChild()
		cc3 := parallelCounter.GetChild()
		cc4 := parallelCounter.GetChild()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc1.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc2.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc3.Count()
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc4.Count()
			}
		})
	})

	// parallel count 500
	b.Run("atomicCounter parallel-4 count 500", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomicCounter.CountN(500)
			}
		})
	})
	b.Run("rotateCounter parallel-4 count 500", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				rotateCounter.CountN(500)
			}
		})
	})
	b.Run("childCounter parallel-4 count 500", func(b *testing.B) {
		cc1 := parallelCounter.GetChild()
		cc2 := parallelCounter.GetChild()
		cc3 := parallelCounter.GetChild()
		cc4 := parallelCounter.GetChild()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc1.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc2.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc3.CountN(500)
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cc4.CountN(500)
			}
		})
	})

}
