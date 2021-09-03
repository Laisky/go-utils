package utils

import "skiplist"

func NewSkiplist() *skiplist.SkipList {
	return skiplist.New()
}
