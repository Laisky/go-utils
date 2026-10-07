package utils

import (
	"testing"
)

type Item struct {
	k string
	v int
}

// GetValue returns the item's integer sort key v, satisfying SortItemItf so PairList can order items by it.
func (i *Item) GetValue() int {
	return i.v
}

// GetData returns the item's string payload k as an any value, satisfying SortItemItf.
func (i *Item) GetData() any {
	return i.k
}

// TestSortSmallest verifies that SortSmallest sorts a PairList in place in ascending order of GetValue, so the
// first three items hold the values 1, 15, and 22.
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

// TestSortBiggest verifies that SortBiggest sorts a PairList in place in descending order of GetValue, so the
// first three items hold the values 40992, 932, and 99.
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
