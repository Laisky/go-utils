package journal_test

import (
	"math/rand"
	"testing"

	"github.com/Laisky/go-utils/journal"
)

func TestNewInt64Set(t *testing.T) {
	s := journal.NewInt64Set()
	for i := int64(0); i < 10; i++ {
		s.Add(i)
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

func BenchmarkInt64Set(b *testing.B) {
	s := journal.NewInt64Set()
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.Add(rand.Int63())
			}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s.CheckAndRemove(rand.Int63())
			}
		})
	})
	b.Run("add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.Add(rand.Int63())
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
}
