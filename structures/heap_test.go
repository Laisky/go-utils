package structures_test

import (
	"container/heap"
	"fmt"
	"math/rand"
	"testing"

	"github.com/Laisky/go-utils/structures"
)

type HeapItemQ []structures.HeapItemItf

// Item item that need to sort
type Item struct {
	p int
	k interface{}
}

// GetKey get key of item
func (it *Item) GetKey() interface{} {
	return it.k
}

// GetPriority get priority of item
func (it *Item) GetPriority() int {
	return it.p
}

var (
	itemsWaitToSort = HeapItemQ{
		&Item{p: 1},
		&Item{p: 3},
		&Item{p: 55},
		&Item{p: 2},
		&Item{p: 4441},
		&Item{p: 15555},
		&Item{p: 122},
	}
)

func ExampleGetLargestNItems() {
	var (
		itemsWaitToSort = HeapItemQ{
			&Item{p: 1},
			&Item{p: 3},
			&Item{p: 55},
			&Item{p: 2},
			&Item{p: 4441},
			&Item{p: 15555},
			&Item{p: 122},
		}
		itemChan = make(chan structures.HeapItemItf)
	)

	go func() {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}()

	items, err := structures.GetLargestNItems(itemChan, 3)
	if err != nil {
		panic(err)
	}

	for _, item := range items {
		// 15555
		// 4441
		// 112
		fmt.Println(item.GetPriority())
	}
}

func ExampleGetSmallestNItems() {
	var (
		itemsWaitToSort = HeapItemQ{
			&Item{p: 1},
			&Item{p: 3},
			&Item{p: 55},
			&Item{p: 2},
			&Item{p: 4441},
			&Item{p: 15555},
			&Item{p: 122},
		}
		itemChan = make(chan structures.HeapItemItf)
	)

	go func() {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}()

	items, err := structures.GetSmallestNItems(itemChan, 3)
	if err != nil {
		panic(err)
	}

	for _, item := range items {
		// 1
		// 2
		// 3
		fmt.Println(item.GetPriority())
	}
}

func TestGetTopKItems(t *testing.T) {
	// defer utils.Logger.Sync()
	generate := func(itemChan chan structures.HeapItemItf) {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}

	var (
		items    HeapItemQ
		err      error
		itemChan chan structures.HeapItemItf
	)

	// test highest
	itemChan = make(chan structures.HeapItemItf)
	go generate(itemChan)
	items, err = structures.GetTopKItems(itemChan, 3, true)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if items[0].GetPriority() != 15555 {
		t.Errorf("expect 15555, got %+v", items[0].GetPriority())
	}
	if items[1].GetPriority() != 4441 {
		t.Errorf("expect 4441, got %+v", items[1].GetPriority())
	}
	if items[2].GetPriority() != 122 {
		t.Errorf("expect 122, got %+v", items[2].GetPriority())
	}

	// test lowest
	itemChan = make(chan structures.HeapItemItf)
	go generate(itemChan)
	items, err = structures.GetTopKItems(itemChan, 3, false)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if items[0].GetPriority() != 1 {
		t.Errorf("expect 1, got %+v", items[0].GetPriority())
	}
	if items[1].GetPriority() != 2 {
		t.Errorf("expect 2, got %+v", items[1].GetPriority())
	}
	if items[2].GetPriority() != 3 {
		t.Errorf("expect 3, got %+v", items[2].GetPriority())
	}
}

func TestPriorityQ(t *testing.T) {
	for _, isMaxTop := range []bool{true, false} {
		q := structures.NewPriorityQ(isMaxTop)
		heap.Init(q)
		var (
			v, n int
		)
		for i := 0; i < 10000; i++ {
			n = rand.Intn(100)
			if n < 50 {
				v = rand.Intn(1000)
				heap.Push(q, &Item{
					p: v,
					k: v,
				})
			} else if n < 75 {
				v = rand.Intn(1000)
				q.Remove(&Item{
					p: v,
					k: v,
				})
				heap.Init(q)
			} else {
				if q.Len() > 0 {
					heap.Pop(q)
				}
			}
		}

		heap.Push(q, &Item{
			p: 0,
			k: 0,
		})
		heap.Push(q, &Item{
			p: 1000,
			k: 1000,
		})

		results := make([]int, q.Len())[:0]
		var lastP, curP int
		for {
			if q.Len() == 0 {
				break
			}
			curP = heap.Pop(q).(*Item).GetPriority()
			if lastP != 0 {
				if isMaxTop && curP > lastP {
					t.Errorf("%v should <= %v", curP, lastP)
				} else if !isMaxTop && curP < lastP {
					t.Errorf("%v should >= %v", curP, lastP)
				}
			}

			lastP = curP
			results = append(results, curP)
		}
		t.Logf("%v[%v]: %v\n", isMaxTop, len(results), results[:10])
	}
	// t.Error("done")
}

func TestLimitSizeHeap(t *testing.T) {
	heap, err := structures.NewLimitSizeHeap(5, true)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	var (
		it structures.HeapItemItf
		n  int
	)
	for i := 0; i < 100; i++ {
		n = rand.Intn(1000)
		it = heap.Push(&Item{
			p: n,
			k: n,
		})
		if it != nil {
			t.Logf("push %v, pop %v", n, it.GetPriority())
		} else {
			t.Logf("push %v", n)
		}
	}

	var oldit structures.HeapItemItf
	results := []int{}
	for {
		if it = heap.Pop(); it == nil {
			break
		}
		results = append(results, it.GetPriority())
		if oldit != nil {
			if oldit.GetPriority() > it.GetPriority() {
				t.Fatal(oldit.GetPriority(), "should <=", it.GetPriority(), ",", results)
			}
		}
		oldit = it
	}

	t.Log("results: ", results)
	// t.Error("done")
}

func BenchmarkLimitSizeHeap(b *testing.B) {
	heap5, err := structures.NewLimitSizeHeap(5, true)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	heap50, err := structures.NewLimitSizeHeap(50, true)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	heap500, err := structures.NewLimitSizeHeap(500, true)
	if err != nil {
		b.Fatalf("%+v", err)
	}

	var n int
	b.Run("heap 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n = rand.Intn(1000)
			heap5.Push(&Item{
				p: n,
				k: n,
			})
		}
	})
	b.Run("heap 50", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n = rand.Intn(1000)
			heap50.Push(&Item{
				p: n,
				k: n,
			})
		}
	})
	b.Run("heap 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n = rand.Intn(1000)
			heap500.Push(&Item{
				p: n,
				k: n,
			})
		}
	})

}
