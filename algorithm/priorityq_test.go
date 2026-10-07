package algorithm

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Laisky/go-utils/v6/common"
)

// newItem is a test helper that builds a PriorityItem with value val and label name.
func newItem(val int, name string) PriorityItem[int] {
	return PriorityItem[int]{Val: val, Name: name}
}

// TestPriorityQ_AscOrder verifies that an ascending PriorityQ starts empty with a nil Peek,
// peeks the smallest pushed value, pops values in ascending order, and returns nil from Peek
// once drained.
func TestPriorityQ_AscOrder(t *testing.T) {
	pq := NewPriorityQ[int](common.SortOrderAsc)

	// Initially empty
	assert.Equal(t, 0, pq.Len())
	assert.Nil(t, pq.Peek())

	// Push items
	pq.Push(newItem(5, "five"))
	pq.Push(newItem(1, "one"))
	pq.Push(newItem(3, "three"))

	assert.Equal(t, 3, pq.Len())

	// Peek should give smallest (asc order)
	peek := pq.Peek()
	assert.Equal(t, 1, peek.GetVal())

	// Pop should return items in ascending order
	vals := []int{}
	for pq.Len() > 0 {
		item := pq.Pop()
		vals = append(vals, item.GetVal())
	}
	assert.Equal(t, []int{1, 3, 5}, vals)

	// After popping all, Peek should be nil
	assert.Nil(t, pq.Peek())
}

// TestPriorityQ_DescOrder verifies that a descending PriorityQ peeks the largest pushed value,
// pops values in descending order, and returns nil from Peek once drained.
func TestPriorityQ_DescOrder(t *testing.T) {
	pq := NewPriorityQ[int](common.SortOrderDesc)

	pq.Push(newItem(5, "five"))
	pq.Push(newItem(1, "one"))
	pq.Push(newItem(3, "three"))

	assert.Equal(t, 3, pq.Len())

	// Peek should give largest (desc order)
	peek := pq.Peek()
	assert.Equal(t, 5, peek.GetVal())

	// Pop should return items in descending order
	vals := []int{}
	for pq.Len() > 0 {
		item := pq.Pop()
		vals = append(vals, item.GetVal())
	}
	assert.Equal(t, []int{5, 3, 1}, vals)

	// After popping all, Peek should be nil
	assert.Nil(t, pq.Peek())
}

// TestPriorityItem_GetVal verifies that GetVal returns the value stored in a PriorityItem and
// that its Name field is preserved.
func TestPriorityItem_GetVal(t *testing.T) {
	item := newItem(42, "answer")
	assert.Equal(t, 42, item.GetVal())
	assert.Equal(t, "answer", item.Name)
}

// TestInnerPriorityQ_PushPop verifies that the internal ascending queue satisfies
// container/heap: heap.Push grows it and heap.Pop returns the smallest value first.
func TestInnerPriorityQ_PushPop(t *testing.T) {
	pq := newInnerPriorityQ[int](common.SortOrderAsc)

	// Push via heap interface
	heap.Push(pq, newItem(10, "ten"))
	heap.Push(pq, newItem(2, "two"))
	heap.Push(pq, newItem(7, "seven"))

	assert.Equal(t, 3, pq.Len())

	// Pop via heap interface
	item := heap.Pop(pq).(PriorityItemItf[int])
	assert.Equal(t, 2, item.GetVal())

	// Remaining items
	assert.Equal(t, 2, pq.Len())
}

// TestPeekEmptyQueue verifies that Peek on a newly created PriorityQ returns nil.
func TestPeekEmptyQueue(t *testing.T) {
	pq := NewPriorityQ[int](common.SortOrderAsc)
	assert.Nil(t, pq.Peek())
}

// TestMixedOperations verifies that interleaved Push, Peek, and Pop calls on an ascending
// PriorityQ keep Len accurate and still pop the remaining values in ascending order.
func TestMixedOperations(t *testing.T) {
	pq := NewPriorityQ[int](common.SortOrderAsc)

	pq.Push(newItem(4, "four"))
	pq.Push(newItem(2, "two"))
	assert.Equal(t, 2, pq.Len())

	peek := pq.Peek()
	assert.Equal(t, 2, peek.GetVal())

	pop := pq.Pop()
	assert.Equal(t, 2, pop.GetVal())
	assert.Equal(t, 1, pq.Len())

	pq.Push(newItem(1, "one"))
	pq.Push(newItem(3, "three"))

	// Final order should be 1,3,4
	vals := []int{}
	for pq.Len() > 0 {
		vals = append(vals, pq.Pop().GetVal())
	}
	assert.Equal(t, []int{1, 3, 4}, vals)
}
