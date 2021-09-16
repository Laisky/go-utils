package utils

import skiplist "github.com/Laisky/fast-skiplist"

func NewSkiplist() *skiplist.SkipList {
	return skiplist.New()
}
