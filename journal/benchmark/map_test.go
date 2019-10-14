package journal_test

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/RoaringBitmap/roaring"

	"github.com/Laisky/go-utils"
	// mapset "github.com/deckarep/golang-set"
)

func BenchmarkMap(b *testing.B) {
	m := map[string]struct{}{}
	sm := sync.Map{}
	// s := mapset.NewSet()
	rm := roaring.New()
	var k string
	b.Run("map add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m[utils.RandomStringWithLength(20)] = struct{}{}
		}
	})
	b.Run("sync map add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm.Store(utils.RandomStringWithLength(20), struct{}{})
		}
	})
	b.Run("bitmap add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rm.AddInt(rand.Int())
		}
	})
	// b.Run("set add", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		s.Add(utils.RandomStringWithLength(20))
	// 	}
	// })
	b.Run("map get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = utils.RandomStringWithLength(20)
			_ = m[k]
			delete(m, k)
		}
	})
	b.Run("sync map get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = utils.RandomStringWithLength(20)
			sm.Load(k)
			sm.Delete(k)
		}
	})
	b.Run("bitmap get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rm.ContainsInt(rand.Int())
			rm.Remove(rand.Uint32())
		}
	})
	// b.Run("set get", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		k = utils.RandomStringWithLength(20)
	// 		s.Contains(k)
	// 		s.Remove(k)
	// 	}
	// })
}

func BenchmarkSet(b *testing.B) {
	s1 := &sync.Map{}
	s2 := &sync.Map{}
	load := struct{}{}
	var k int
	b.Run("simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s1.Store(rand.Int(), load)
		}
	})
	b.Run("simple remove", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = rand.Int()
			s1.Load(k)
			s1.Delete(k)
		}
	})
	b.Run("bool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s2.Store(rand.Int(), true)
		}
	})
	b.Run("bool remove", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = rand.Int()
			s2.Load(k)
			s2.Store(k, false)
		}
	})
}
