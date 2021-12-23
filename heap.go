package utils

import (
	"container/heap"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

// itemType item that need to sort
type itemType struct {
	priority int
	key      interface{}
}

// GetKey get key of item
func (it *itemType) GetKey() interface{} {
	return it.key
}

// GetPriority get priority of item
func (it *itemType) GetPriority() int {
	return it.priority
}

type HeapSlice []HeapItemItf

// PriorityQ lower structure used by heap
//
// do not use this structure directly
type PriorityQ struct {
	isMaxTop bool
	q        []HeapItemItf
}

// NewPriorityQ create new PriorityQ
func NewPriorityQ(isMaxTop bool) *PriorityQ {
	// Logger.Debug("create PriorityQ")
	return &PriorityQ{
		isMaxTop: isMaxTop,
		q:        []HeapItemItf{},
	}
}

// Len get length of items in heapq
func (p *PriorityQ) Len() int {
	// Logger.Debug("len", zap.Int("len", len(p.q)))
	return len(p.q)
}

// Less compare two items in heapq
func (p *PriorityQ) Less(i, j int) bool {
	// Logger.Debug("less two items", zap.Int("i", i), zap.Int("j", j))
	if p.isMaxTop {
		return p.q[i].GetPriority() > p.q[j].GetPriority()
	}

	return p.q[i].GetPriority() < p.q[j].GetPriority()
}

// Swap swat two items in heapq
func (p *PriorityQ) Swap(i, j int) {
	// Logger.Debug("swap two items", zap.Int("i", i), zap.Int("j", j))
	p.q[i], p.q[j] = p.q[j], p.q[i]
}

// Push push new item into heapq
func (p *PriorityQ) Push(x interface{}) {
	// Logger.Debug("push item", zap.Int("priority", x.(HeapItemItf).GetPriority()))
	item := x.(HeapItemItf)
	p.q = append(p.q, item)
}

// Remove remove an specific item
func (p *PriorityQ) Remove(v HeapItemItf) (ok bool) {
	for i, it := range p.q {
		if it == v {
			p.q = append(p.q[:i], p.q[i+1:]...)
			return true
		}
	}

	return false
}

// Pop pop from the tail.
// if `isMaxTop=True`, pop the biggest item
func (p *PriorityQ) Pop() (popped interface{}) {
	Logger.Debug("pop item")
	n := len(p.q)
	popped = p.q[n-1]
	p.q[n-1] = nil // avoid memory leak
	p.q = p.q[:n-1]
	return popped
}

// HeapItemItf items need to sort
type HeapItemItf interface {
	GetKey() interface{}
	GetPriority() int
}

// GetLargestNItems get N highest priority items
func GetLargestNItems(inputChan <-chan HeapItemItf, topN int) ([]HeapItemItf, error) {
	return GetTopKItems(inputChan, topN, false)
}

// GetSmallestNItems get N smallest priority items
func GetSmallestNItems(inputChan <-chan HeapItemItf, topN int) ([]HeapItemItf, error) {
	return GetTopKItems(inputChan, topN, true)
}

// GetTopKItems calculate topN by heap
//
//   * use min-heap to calculates topN Highest items.
//   * use max-heap to calculates topN Lowest items.
func GetTopKItems(inputChan <-chan HeapItemItf, topN int, isHighest bool) ([]HeapItemItf, error) {
	Logger.Debug("GetMostFreqWords for key2PriMap", zap.Int("topN", topN))
	if topN < 2 {
		return nil, errors.Errorf("GetMostFreqWords topN must larger than 2")
	}

	var (
		i               int
		ok              bool
		item, thresItem HeapItemItf
		items           = make([]HeapItemItf, topN)
		nTotal          = 0
		p               = NewPriorityQ(!isHighest)
	)

LOAD_LOOP:
	for i = 0; i < topN; i++ { // load first topN items
		item, ok = <-inputChan
		if !ok { // channel closed
			inputChan = nil
			break LOAD_LOOP
		}
		nTotal++
		// is `isHighest=true`, thresItem is the smallest item
		// is `isHighest=false`, thresItem is the biggest item
		if thresItem == nil ||
			(isHighest && item.GetPriority() < thresItem.GetPriority()) ||
			(!isHighest && item.GetPriority() > thresItem.GetPriority()) {
			thresItem = item
		}

		p.Push(&itemType{
			key:      item.GetKey(),
			priority: item.GetPriority(),
		})
	}

	if inputChan == nil {
		if p.Len() == 1 { // only one item
			return []HeapItemItf{item}, nil
		}
		if p.Len() == 0 {
			return []HeapItemItf{}, nil
		}
	}

	heap.Init(p) // initialize heap

	// load all remain items
	if inputChan != nil {
		for item = range inputChan {
			nTotal++
			if (isHighest && item.GetPriority() <= thresItem.GetPriority()) ||
				(!isHighest && item.GetPriority() >= thresItem.GetPriority()) {
				continue
			}

			heap.Push(p, &itemType{
				priority: item.GetPriority(),
				key:      item.GetKey(),
			})
			thresItem = heap.Pop(p).(*itemType)
		}
	}

	Logger.Debug("process all items", zap.Int("total", nTotal))
	for i := 1; i <= topN; i++ { // pop all needed items
		item = heap.Pop(p).(*itemType)
		items[topN-i] = item
	}

	return items, nil
}

// LimitSizeHeap heap with limit size
type LimitSizeHeap struct {
	q             *PriorityQ
	thresItem     HeapItemItf
	isHighest     bool
	size, maxSize int64
}

// NewLimitSizeHeap create new LimitSizeHeap
func NewLimitSizeHeap(size int, isHighest bool) (h *LimitSizeHeap, err error) {
	if size < 1 {
		return nil, errors.Errorf("size must greater than 0")
	}

	h = &LimitSizeHeap{
		q:         NewPriorityQ(!isHighest),
		maxSize:   int64(size),
		isHighest: isHighest,
	}
	heap.Init(h.q)
	return
}

// Push push item into heap, return popped item if exceed size
func (h *LimitSizeHeap) Push(item HeapItemItf) HeapItemItf {
	if h.size == h.maxSize && h.thresItem != nil {
		if h.isHighest && item.GetPriority() <= h.thresItem.GetPriority() {
			return item // item <= minimal member
		} else if !h.isHighest && item.GetPriority() >= h.thresItem.GetPriority() {
			return item // item >= maximal member
		}
	}

	// update thresItem
	if h.thresItem == nil {
		h.thresItem = item
	} else if h.isHighest && item.GetPriority() < h.thresItem.GetPriority() {
		h.thresItem = item
	} else if !h.isHighest && item.GetPriority() > h.thresItem.GetPriority() {
		h.thresItem = item
	}

	h.size++
	heap.Push(h.q, item)
	if h.size > h.maxSize {
		h.size--
		h.thresItem = heap.Pop(h.q).(HeapItemItf)
		return h.thresItem
	}

	return nil
}

// Pop pop from the tail.
// if `isHighest=True`, pop the biggest item
func (h *LimitSizeHeap) Pop() HeapItemItf {
	if h.size == 0 {
		return nil
	}

	h.size--
	return heap.Pop(h.q).(HeapItemItf)
}
