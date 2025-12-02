package algorithm

import (
	"container/heap"
	"testing"

	"github.com/Laisky/go-utils/v6/common"
	"github.com/stretchr/testify/assert"
)

// helper function to create items
func newItem(val int, name string) PriorityItem[int] {
	return PriorityItem[int]{Val: val, Name: name}
}

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

func TestPriorityItem_GetVal(t *testing.T) {
	item := newItem(42, "answer")
	assert.Equal(t, 42, item.GetVal())
	assert.Equal(t, "answer", item.Name)
}

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

func TestPeekEmptyQueue(t *testing.T) {
	pq := NewPriorityQ[int](common.SortOrderAsc)
	assert.Nil(t, pq.Peek())
}

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
