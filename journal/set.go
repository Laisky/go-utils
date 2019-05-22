package journal

import "sync"

type Int64Set struct {
	padding struct{}
	d       *sync.Map
}

func NewInt64Set() *Int64Set {
	return &Int64Set{
		padding: struct{}{},
		d:       &sync.Map{},
	}
}

func (s *Int64Set) Add(i int64) {
	s.d.Store(i, s.padding)
}

func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
	_, ok = s.d.Load(i)
	s.d.Delete(i)
	return ok
}
