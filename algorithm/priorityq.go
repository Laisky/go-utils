package algorithm

import (
	"container/heap"

	"github.com/Laisky/go-utils/v4/common"
)

// PriorityQ priority queue
//
// Do not use this structure directly, use `NewPriorityQ` instead.
type PriorityQ[T common.Sortable] struct {
	q *innerPriorityQ[T]
}

// PriotiryItemItf priority item interface
type PriotiryItemItf[T common.Sortable] interface {
	GetVal() T
}

// PriorityItem priority item
type PriorityItem[T common.Sortable] struct {
	// Val 	T
	Val T
	// Name whatever to identify this item
	Name any
}

// GetVal get value of priority item
func (t PriorityItem[T]) GetVal() T {
	return t.Val
}

// NewPriorityQ create new PriorityQ
//
// Args:
//   - order: common.SortOrderAsc or common.SortOrderDesc,
//     if you want to get topN items, use common.SortOrderDesc,
//     if you want to get bottomN items, use common.SortOrderAsc.
func NewPriorityQ[T common.Sortable](order common.SortOrder) *PriorityQ[T] {
	return &PriorityQ[T]{
		q: newinnerPriorityQ[T](order),
	}
}

// Push push item into priority queue
func (pq *PriorityQ[T]) Push(v PriotiryItemItf[T]) {
	heap.Push(pq.q, v)
}

// Pop pop item from priority queue
func (pq *PriorityQ[T]) Pop() PriotiryItemItf[T] {
	return heap.Pop(pq.q).(PriotiryItemItf[T]) //nolint:forcetypeassert // panic
}

// Len return length of priority queue
func (pq *PriorityQ[T]) Len() int {
	return pq.q.Len()
}

// Peek peek item from priority queue
func (pq *PriorityQ[T]) Peek() PriotiryItemItf[T] {
	return pq.q.vals[len(pq.q.vals)-1]
}

// // LimitSizePriorityQ priority queue with limit size
// //
// // Do not use this structure directly, use `NewLimitSizePriorityQ` instead.
// type LimitSizePriorityQ[T common.Sortable] struct {
// 	*PriorityQ[T]
// 	limit int
// }

// // NewLimitSizePriorityQ create new LimitSizePriorityQ
// func NewLimitSizePriorityQ[T common.Sortable](order common.SortOrder, limit int) *LimitSizePriorityQ[T] {
// 	return &LimitSizePriorityQ[T]{
// 		PriorityQ: NewPriorityQ[T](order),
// 		limit:     limit,
// 	}
// }

// // Push push item into priority queue
// func (pq *LimitSizePriorityQ[T]) Push(item heapItem[T]) {
// 	if pq.Len() >= pq.limit {
// 		heap.Push(pq.q, item)
// 		heap.Pop(pq.q)
// 	} else {
// 		heap.Push(pq.q, item)
// 	}
// }

// // Pop pop item from priority queue
// func (pq *LimitSizePriorityQ[T]) Pop() heapItem[T] {
// 	return heap.Pop(pq.q).(heapItem[T])
// }

// // Len return length of priority queue
// func (pq *LimitSizePriorityQ[T]) Len() int {
// 	return pq.q.Len()
// }

// // Peek peek item from priority queue
// func (pq *LimitSizePriorityQ[T]) Peek() heapItem[T] {
// 	return pq.q.vals[0]
// }

// A innerPriorityQ implements heap.Interface and holds Items.
//
// Do not use this structure directly, use `NewPriorityQueue` instead.
type innerPriorityQ[T common.Sortable] struct {
	vals  []PriotiryItemItf[T]
	order common.SortOrder
}

// newinnerPriorityQ create new PriorityQ
//
// https://pkg.go.dev/container/heap@go1.21.5#example-package-IntHeap
func newinnerPriorityQ[T common.Sortable](order common.SortOrder) *innerPriorityQ[T] {
	return &innerPriorityQ[T]{
		vals:  []PriotiryItemItf[T]{},
		order: order,
	}
}

// Len is the number of elements in the collection.
func (pq *innerPriorityQ[T]) Len() int { return len(pq.vals) }

// Less compare two items in heapq
func (pq *innerPriorityQ[T]) Less(i, j int) bool {
	if pq.order == common.SortOrderAsc {
		return pq.vals[i].GetVal() < pq.vals[j].GetVal()
	} else {
		return pq.vals[i].GetVal() > pq.vals[j].GetVal()
	}
}

// Swap swap two items in heapq
func (pq *innerPriorityQ[T]) Swap(i, j int) {
	pq.vals[i], pq.vals[j] = pq.vals[j], pq.vals[i]
	// pq.vals[i].heapIdx = i
	// pq.vals[j].heapIdx = j
}

func (pq *innerPriorityQ[T]) Push(v any) {
	pq.vals = append(pq.vals, v.(PriotiryItemItf[T])) //nolint:forcetypeassert // panic
}

// Pop pop item from heapq
func (pq *innerPriorityQ[T]) Pop() any {
	n := len(pq.vals)
	item := pq.vals[n-1]
	clear(pq.vals[n-1:]) // avoid memory leak
	pq.vals = pq.vals[0 : n-1]
	// item.heapIdx = -1 // for safety
	return item
}
