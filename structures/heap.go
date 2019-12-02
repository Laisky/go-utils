package structures

import (
	"container/heap"
	"fmt"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
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

// PriorityQ priority queue based heap
type PriorityQ struct {
	descend bool
	q       []HeapItemItf
}

// NewPriorityQ create new heapq
func NewPriorityQ(descend bool) *PriorityQ {
	utils.Logger.Debug("create PriorityQ")
	return &PriorityQ{
		descend: descend,
		q:       []HeapItemItf{},
	}
}

// Len get length of items in heapq
func (p *PriorityQ) Len() (n int) {
	utils.Logger.Debug("len", zap.Int("len", len(p.q)))
	n = len(p.q)
	return
}

// Less compare two items in heapq
func (p *PriorityQ) Less(i, j int) bool {
	utils.Logger.Debug("less two items", zap.Int("i", i), zap.Int("j", j))
	if p.descend {
		return p.q[i].GetPriority() < p.q[j].GetPriority()
	}

	return p.q[i].GetPriority() > p.q[j].GetPriority()
}

// Swap swat two items in heapq
func (p *PriorityQ) Swap(i, j int) {
	utils.Logger.Debug("swap two items", zap.Int("i", i), zap.Int("j", j))
	p.q[i], p.q[j] = p.q[j], p.q[i]
}

// Push push new item into heapq
func (p *PriorityQ) Push(x interface{}) {
	utils.Logger.Debug("push item", zap.Int("priority", x.(HeapItemItf).GetPriority()))
	item := x.(HeapItemItf)
	p.q = append(p.q, item)
}

// Pop pop highest priority item
func (p *PriorityQ) Pop() (popped interface{}) {
	utils.Logger.Debug("pop item")
	n := len(p.q)
	popped = p.q[n-1]
	p.q = p.q[:n-1]
	return
}

// HeapItemItf items need to sort
type HeapItemItf interface {
	GetKey() interface{}
	GetPriority() int
}

// GetLargestNItems get N highest priority items
func GetLargestNItems(inputChan <-chan HeapItemItf, topN int) ([]HeapItemItf, error) {
	return GetTopKItems(inputChan, topN, true)
}

// GetSmallestNItems get N smallest priority items
func GetSmallestNItems(inputChan <-chan HeapItemItf, topN int) ([]HeapItemItf, error) {
	return GetTopKItems(inputChan, topN, false)
}

// GetTopKItems calculate topN by heap
// descend=true: use min-heap to calculates topN Highest items
// descend=false: use max-heap to calculates topN Lowest items
func GetTopKItems(inputChan <-chan HeapItemItf, topN int, descend bool) ([]HeapItemItf, error) {
	utils.Logger.Debug("GetMostFreqWords for key2PriMap", zap.Int("topN", topN))
	if topN < 2 {
		return nil, fmt.Errorf("GetMostFreqWords topN must larger than 2")
	}

	var (
		i               int
		ok              bool
		item, thresItem HeapItemItf
		items           = make([]HeapItemItf, topN)
		nTotal          = 0
		p               = NewPriorityQ(descend)
	)

LOAD_LOOP:
	for i = 0; i < topN; i++ { // load first topN items
		item, ok = <-inputChan
		if !ok { // channel closed
			inputChan = nil
			break LOAD_LOOP
		}
		nTotal++
		if thresItem == nil ||
			(descend && item.GetPriority() < thresItem.GetPriority()) ||
			(!descend && item.GetPriority() > thresItem.GetPriority()) {
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
			if (descend && item.GetPriority() <= thresItem.GetPriority()) ||
				(!descend && item.GetPriority() >= thresItem.GetPriority()) {
				continue
			}

			heap.Push(p, &itemType{
				priority: item.GetPriority(),
				key:      item.GetKey(),
			})
			thresItem = heap.Pop(p).(*itemType)
		}
	}

	utils.Logger.Debug("process all items", zap.Int("total", nTotal))
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
	descend       bool
	size, maxSize int64
}

// NewLimitSizeHeap create new LimitSizeHeap
func NewLimitSizeHeap(size int, descend bool) (h *LimitSizeHeap, err error) {
	if size < 1 {
		return nil, fmt.Errorf("size must greater than 0")
	}

	h = &LimitSizeHeap{
		q:       NewPriorityQ(descend),
		maxSize: int64(size),
		descend: descend,
	}
	heap.Init(h.q)
	return
}

// Push push item into heap, return popped item if exceed size
func (h *LimitSizeHeap) Push(item HeapItemItf) HeapItemItf {
	if h.size == h.maxSize && h.thresItem != nil {
		if h.descend && item.GetPriority() <= h.thresItem.GetPriority() {
			return item // item <= minimal member
		} else if !h.descend && item.GetPriority() >= h.thresItem.GetPriority() {
			return item // item >= maximal member
		}
	}

	if h.thresItem == nil {
		h.thresItem = item
	} else if h.descend && item.GetPriority() < h.thresItem.GetPriority() {
		h.thresItem = item
	} else if !h.descend && item.GetPriority() > h.thresItem.GetPriority() {
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

func (h *LimitSizeHeap) Pop() HeapItemItf {
	if h.size == 0 {
		return nil
	}

	h.size--
	return h.q.Pop().(HeapItemItf)
}
