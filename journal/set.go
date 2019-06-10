package journal

import (
	"math"
	"sync"

	"github.com/RoaringBitmap/roaring"
)

// Uint32Set set depends on bitmap.
// only support uint32, so cannot support more than 4294967295 numbers.
type Uint32Set struct {
	sync.Mutex
	padding struct{}
	d       *roaring.Bitmap
}

// NewUint32Set create new Uint32Set
func NewUint32Set() *Uint32Set {
	s := &Uint32Set{
		padding: struct{}{},
		d:       roaring.NewBitmap(),
	}
	return s
}

// AddInt64 add new number
func (s *Uint32Set) AddInt64(i int64) {
	s.Lock()
	defer s.Unlock()
	s.d.Add(uint32(i % math.MaxUint32))
}

// CheckAndRemoveInt64 return true if exists
func (s *Uint32Set) CheckAndRemoveInt64(i int64) (ok bool) {
	s.Lock()
	defer s.Unlock()
	return s.d.CheckedRemove(uint32(i % math.MaxUint32))
}

// AddUint32 add new number
func (s *Uint32Set) AddUint32(i uint32) {
	s.Lock()
	defer s.Unlock()
	s.d.Add(i)
}

// CheckAndRemoveUint32 return true if exists
func (s *Uint32Set) CheckAndRemoveUint32(i uint32) (ok bool) {
	s.Lock()
	defer s.Unlock()
	return s.d.CheckedRemove(i)
}

// GetLen (deprecated) return length
func (s *Uint32Set) GetLen() int {
	return int(s.d.GetCardinality())
}

// Int64Set set depends on sync.Map.
// cost much more memory than bitmap
type Int64Set struct {
	padding struct{}
	d       *sync.Map
}

// NewInt64Set create new Int64Set
func NewInt64Set() *Int64Set {
	return &Int64Set{
		padding: struct{}{},
		d:       &sync.Map{},
	}
}

// Add add new number
func (s *Int64Set) Add(i int64) {
	s.d.Store(i, s.padding)
}

// CheckAndRemove return true if exists
func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
	_, ok = s.d.Load(i)
	s.d.Delete(i)
	return ok
}

// GetLen return length
func (s *Int64Set) GetLen() int {
	l := 0
	s.d.Range(func(k, v interface{}) bool {
		l++
		return true
	})
	return l
}
