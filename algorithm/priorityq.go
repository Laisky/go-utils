package algorithm

import (
	"container/heap"

	"github.com/Laisky/go-utils/v6/common"
)

// PriorityQ is a generic priority queue.
// Do not use this structure directly, use `NewPriorityQ` instead.
type PriorityQ[T common.Sortable] struct {
	q *innerPriorityQ[T]
}

// PriorityItemItf defines the interface for items stored in the priority queue.
type PriorityItemItf[T common.Sortable] interface {
	GetVal() T
}

// PriorityItem is a concrete implementation of PriorityItemItf.
type PriorityItem[T common.Sortable] struct {
	Val  T   // Value used for ordering
	Name any // Optional identifier
}

// GetVal returns the value of the priority item.
func (t PriorityItem[T]) GetVal() T {
	return t.Val
}

// NewPriorityQ creates a new PriorityQ.
//
// Args:
//   - order: common.SortOrderAsc or common.SortOrderDesc.
//     Use common.SortOrderDesc for top-N items,
//     Use common.SortOrderAsc for bottom-N items.
func NewPriorityQ[T common.Sortable](order common.SortOrder) *PriorityQ[T] {
	return &PriorityQ[T]{
		q: newInnerPriorityQ[T](order),
	}
}

// Push inserts an item into the priority queue.
func (pq *PriorityQ[T]) Push(v PriorityItemItf[T]) {
	heap.Push(pq.q, v)
}

// Pop removes and returns the highest-priority item.
func (pq *PriorityQ[T]) Pop() PriorityItemItf[T] {
	return heap.Pop(pq.q).(PriorityItemItf[T]) //nolint:forcetypeassert // panic if misuse
}

// Len returns the number of items in the queue.
func (pq *PriorityQ[T]) Len() int {
	return pq.q.Len()
}

// Peek returns the highest-priority item without removing it.
func (pq *PriorityQ[T]) Peek() PriorityItemItf[T] {
	if pq.Len() == 0 {
		return nil
	}
	return pq.q.vals[0]
}

// innerPriorityQ implements heap.Interface and holds items.
// Do not use this structure directly, use `NewPriorityQ` instead.
type innerPriorityQ[T common.Sortable] struct {
	vals  []PriorityItemItf[T]
	order common.SortOrder
}

// newInnerPriorityQ creates a new innerPriorityQ.
func newInnerPriorityQ[T common.Sortable](order common.SortOrder) *innerPriorityQ[T] {
	return &innerPriorityQ[T]{
		vals:  []PriorityItemItf[T]{},
		order: order,
	}
}

// Len returns the number of elements in the collection.
func (pq *innerPriorityQ[T]) Len() int { return len(pq.vals) }

// Less compares two items in the heap.
func (pq *innerPriorityQ[T]) Less(i, j int) bool {
	if pq.order == common.SortOrderAsc {
		return pq.vals[i].GetVal() < pq.vals[j].GetVal()
	}
	return pq.vals[i].GetVal() > pq.vals[j].GetVal()
}

// Swap exchanges two items in the heap.
func (pq *innerPriorityQ[T]) Swap(i, j int) {
	pq.vals[i], pq.vals[j] = pq.vals[j], pq.vals[i]
}

// Push adds an item to the heap.
func (pq *innerPriorityQ[T]) Push(v any) {
	pq.vals = append(pq.vals, v.(PriorityItemItf[T])) //nolint:forcetypeassert
}

// Pop removes and returns the last item from the heap.
func (pq *innerPriorityQ[T]) Pop() any {
	n := len(pq.vals)
	item := pq.vals[n-1]
	pq.vals[n-1] = nil // avoid memory leak
	pq.vals = pq.vals[:n-1]
	return item
}
