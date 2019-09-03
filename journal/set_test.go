package journal_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/Laisky/go-utils"

	"github.com/Laisky/go-utils/journal"
)

func TestNewInt64Set(t *testing.T) {
	s := journal.NewInt64Set()
	for i := int64(0); i < 10; i++ {
		s.AddInt64(i)
	}

	for i := int64(5); i < 10; i++ {
		s.CheckAndRemove(i)
	}

	if !s.CheckAndRemove(3) {
		t.Fatal("should contains 3")
	}
	if s.CheckAndRemove(3) {
		t.Fatal("should not contains 3")
	}
	if s.CheckAndRemove(7) {
		t.Fatal("should not contains 7")
	}
}

func TestInt64SetWithTTL(t *testing.T) {
	utils.SetupLogger("debug")
	ctx := context.Background()
	s := journal.NewInt64SetWithTTL(
		ctx,
		1*time.Second)
	for i := int64(0); i < 10; i++ {
		s.AddInt64(i)
	}

	for i := int64(5); i < 10; i++ {
		s.CheckAndRemove(i)
	}

	if !s.CheckAndRemove(3) {
		t.Fatal("should contains 3")
	}
	if !s.CheckAndRemove(7) {
		t.Fatal("should contains 7")
	}

	time.Sleep(2100 * time.Millisecond)
	if s.CheckAndRemove(3) {
		t.Fatal("should not contains 3")
	}
	if s.CheckAndRemove(7) {
		t.Fatal("should not contains 7")
	}
}

func TestNewUint32Set(t *testing.T) {
	s := journal.NewUint32Set()
	for i := uint32(0); i < 10; i++ {
		s.AddUint32(i)
	}

	for i := uint32(5); i < 10; i++ {
		s.CheckAndRemoveUint32(i)
	}

	if !s.CheckAndRemoveUint32(3) {
		t.Fatal("should contains 3")
	}
	if s.CheckAndRemoveUint32(3) {
		t.Fatal("should not contains 3")
	}
	if s.CheckAndRemoveUint32(7) {
		t.Fatal("should not contains 7")
	}
}

func ExampleInt64Set() {
	s := journal.NewInt64Set()
	s.Add(5)

	s.CheckAndRemove(5) // true
	s.CheckAndRemove(3) // false
}

/*
✗ go test -run=All -bench=Int64SetWithTTL -benchtime=5s -benchmem .
BenchmarkInt64SetWithTTL/add-4           5000000              1387 ns/op             187 B/op          4 allocs/op
BenchmarkInt64SetWithTTL/count-4        500000000               19.7 ns/op             0 B/op          0 allocs/op
BenchmarkInt64SetWithTTL/remove-4       50000000               140 ns/op               0 B/op          0 allocs/op
BenchmarkInt64SetWithTTL/parallel-4      2000000              4139 ns/op             348 B/op          8 allocs/op
*/
func BenchmarkInt64SetWithTTL(b *testing.B) {
	utils.SetupLogger("info")
	ctx := context.Background()
	s := journal.NewInt64SetWithTTL(
		ctx,
		10*time.Second)
	b.Run("add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.AddInt64(rand.Int63())
		}
	})
	b.Run("count", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.GetLen()
		}
	})
	// b.Run("count v2", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		s.GetLenV2()
	// 	}
	// })
	b.Run("remove", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.CheckAndRemove(rand.Int63())
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.AddInt64(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.AddInt64(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.CheckAndRemove(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.CheckAndRemove(rand.Int63())
			}
		})
	})
}

/*
✗ go test -run=All -bench=Int64Set -benchtime=5s -benchmem .
BenchmarkInt64Set/add-4         	 1000000	      1064 ns/op	     170 B/op	       4 allocs/op
BenchmarkInt64Set/count-4       	2000000000	         0.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkInt64Set/remove-4      	10000000	       193 ns/op	       0 B/op	       0 allocs/op
BenchmarkInt64Set/parallel-4    	  500000	      4336 ns/op	     343 B/op	       8 allocs/op
*/
func BenchmarkInt64Set(b *testing.B) {
	s := journal.NewInt64Set()
	b.Run("add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.AddInt64(rand.Int63())
		}
	})
	b.Run("count", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.GetLen()
		}
	})
	// b.Run("count v2", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		s.GetLenV2()
	// 	}
	// })
	b.Run("remove", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.CheckAndRemove(rand.Int63())
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.AddInt64(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.AddInt64(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.CheckAndRemove(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.CheckAndRemove(rand.Int63())
			}
		})
	})
}
