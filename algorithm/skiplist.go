package algorithm

import skiplist "github.com/Laisky/fast-skiplist"

// NewSkiplist new skiplist
//
// https://github.com/sean-public/fast-skiplist
func NewSkiplist() *skiplist.SkipList {
	return skiplist.New()
}
