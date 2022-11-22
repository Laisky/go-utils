package utils

import (
	"sort"

	zap "github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v3/log"
)

// SortBiggest sort from biggest to smallest
func SortBiggest(items PairList) PairList {
	sort.Sort(sort.Reverse(items))
	return items
}

// SortSmallest sort from smallest to biggest
func SortSmallest(items PairList) PairList {
	sort.Sort(items)
	return items
}

// SortItemItf interface of sort item
type SortItemItf interface {
	GetValue() int
	GetData() any
}

// PairList array of sort items
type PairList []SortItemItf

// Len return length of sort items
func (p PairList) Len() int {
	log.Shared.Debug("len", zap.Int("len", len(p)))
	return len(p)
}

// Less compare two items
func (p PairList) Less(i, j int) bool {
	log.Shared.Debug("less compare", zap.Int("i", i), zap.Int("j", j))
	return p[i].GetValue() < p[j].GetValue()
}

// Swap change two items
func (p PairList) Swap(i, j int) {
	log.Shared.Debug("swap", zap.Int("i", i), zap.Int("j", j))
	p[i], p[j] = p[j], p[i]
}
