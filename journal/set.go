package journal

type Int64Set struct {
	padding struct{}
	d       map[int64]struct{}
}

func NewInt64Set() *Int64Set {
	return &Int64Set{
		padding: struct{}{},
		d:       map[int64]struct{}{},
	}
}

func (s *Int64Set) Add(i int64) {
	s.d[i] = s.padding
}

func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
	_, ok = s.d[i]
	delete(s.d, i)
	return ok
}
