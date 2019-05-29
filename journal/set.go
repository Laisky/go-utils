package journal

import (
	"math"

	"github.com/RoaringBitmap/roaring"
)

// Int64Set set depends on bitmap.
// only support uint32, so cannot support more than 4294967295 numbers.
type Int64Set struct {
	padding struct{}
	d       *roaring.Bitmap
}

// NewInt64Set create new Int64Set
func NewInt64Set() *Int64Set {
	s := &Int64Set{
		padding: struct{}{},
		d:       roaring.NewBitmap(),
	}
	return s
}

// Add add new number
func (s *Int64Set) Add(i int64) {
	s.d.Add(uint32(i % math.MaxUint32))
}

// CheckAndRemove return true if exists
func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
	return s.d.CheckedRemove(uint32(i % math.MaxUint32))
}

// GetLen (deprecated) return length
func (s *Int64Set) GetLen() int {
	return 1
}

// // Int64Set set depends on sync.Map.
// // cost much more memory than bitmap
// type Int64Set struct {
// 	padding struct{}
// 	d       *sync.Map
// }

// // NewInt64Set create new Int64Set
// func NewInt64Set() *Int64Set {
// 	return &Int64Set{
// 		padding: struct{}{},
// 		d:       &sync.Map{},
// 	}
// }

// // Add add new number
// func (s *Int64Set) Add(i int64) {
// 	s.d.Store(i, s.padding)
// }

// // CheckAndRemove return true if exists
// func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
// 	_, ok = s.d.Load(i)
// 	s.d.Delete(i)
// 	return ok
// }

// // GetLen return length
// func (s *Int64Set) GetLen() int {
// 	l := 0
// 	s.d.Range(func(k, v interface{}) bool {
// 		l++
// 		return true
// 	})
// 	return l
// }
