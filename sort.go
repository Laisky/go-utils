package utils

import (
	"sort"

	zap "go.uber.org/zap"
)

func SortBiggest(items PairList) PairList {
	sort.Sort(sort.Reverse(items))
	return items
}

func SortSmallest(items PairList) PairList {
	sort.Sort(items)
	return items
}

type SortItemItf interface {
	GetValue() int
	GetKey() interface{}
}

type PairList []SortItemItf

func (p PairList) Len() int {
	Logger.Debug("len", zap.Int("len", len(p)))
	return len(p)
}

func (p PairList) Less(i, j int) bool {
	Logger.Debug("less compare", zap.Int("i", i), zap.Int("j", j))
	return p[i].GetValue() < p[j].GetValue()
}

func (p PairList) Swap(i, j int) {
	Logger.Debug("swap", zap.Int("i", i), zap.Int("j", j))
	p[i], p[j] = p[j], p[i]
}