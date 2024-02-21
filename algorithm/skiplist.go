package algorithm

import skiplist "github.com/Laisky/fast-skiplist/v2"

type SkipList[T skiplist.Sortable] struct {
	*skiplist.SkipList[T]
}

// NewSkiplist new skiplist
//
// https://github.com/sean-public/fast-skiplist
func NewSkiplist[T skiplist.Sortable]() SkipList[T] {
	return SkipList[T]{skiplist.New[T]()}
}
