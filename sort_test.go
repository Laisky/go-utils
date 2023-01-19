package utils

import (
	"testing"
)

type Item struct {
	k string
	v int
}

func (i *Item) GetValue() int {
	return i.v
}

func (i *Item) GetData() any {
	return i.k
}

func TestSortSmallest(t *testing.T) {
	items := PairList{
		&Item{k: "1", v: 1},
		&Item{k: "99", v: 99},
		&Item{k: "40992", v: 40992},
		&Item{k: "22", v: 22},
		&Item{k: "15", v: 15},
		&Item{k: "932", v: 932},
	}

	SortSmallest(items)
	if items[0].GetValue() != 1 {
		t.Errorf("except 1, got %v", items[0].GetValue())
	}
	if items[1].GetValue() != 15 {
		t.Errorf("except 15, got %v", items[0].GetValue())
	}
	if items[2].GetValue() != 22 {
		t.Errorf("except 22, got %v", items[0].GetValue())
	}
}

func TestSortBiggest(t *testing.T) {
	items := PairList{
		&Item{k: "1", v: 1},
		&Item{k: "99", v: 99},
		&Item{k: "40992", v: 40992},
		&Item{k: "22", v: 22},
		&Item{k: "15", v: 15},
		&Item{k: "932", v: 932},
	}

	SortBiggest(items)
	if items[0].GetValue() != 40992 {
		t.Errorf("except 40992, got %v", items[0].GetValue())
	}
	if items[1].GetValue() != 932 {
		t.Errorf("except 932, got %v", items[0].GetValue())
	}
	if items[2].GetValue() != 99 {
		t.Errorf("except 99, got %v", items[0].GetValue())
	}
}
