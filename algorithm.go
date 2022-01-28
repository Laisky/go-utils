package utils

import (
	"container/heap"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"

	skiplist "github.com/Laisky/fast-skiplist"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

func NewSkiplist() *skiplist.SkipList {
	return skiplist.New()
}

// -------------------------------------
// Heap
// -------------------------------------

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

// -------------------------------------
// FIFO
// -------------------------------------

var fifoPool = sync.Pool{
	New: func() interface{} {
		return &fifoNode{
			next: unsafe.Pointer(emptyNode),
		}
	},
}

type fifoNode struct {
	next unsafe.Pointer
	d    interface{}
	// refcnt to avoid ABA problem
	refcnt int32
}

// AddRef add ref count
func (f *fifoNode) AddRef(n int32) int32 {
	return atomic.AddInt32(&f.refcnt, n)
}

// Refcnt get ref count
func (f *fifoNode) Refcnt() int32 {
	return atomic.LoadInt32(&f.refcnt)
}

// FIFO is a lock-free First-In-First-Out queue
//
// paper: https://1drv.ms/b/s!Au45o0W1gVVLuNxYkPzfBo4fOssFPQ?e=TYxHKl
type FIFO struct {
	// head the node that before real head node
	//
	// head.next is the real head node
	//
	// unsafe.pointer will tell gc not to remove object in heap
	head unsafe.Pointer
	// tail maybe(maynot) the tail node in queue
	tail unsafe.Pointer
	len  int64
}

// emptyNode is the default value to unsafe.pointer as an empty pointer
var emptyNode = &fifoNode{
	d: "empty",
}

// NewFIFO create a new FIFO queue
func NewFIFO() *FIFO {
	// add a dummy node to the queue to avoid contention
	// betweet head & tail when queue is empty
	var dummyNode = fifoPool.Get().(*fifoNode)
	dummyNode.d = "dummy"
	dummyNode.next = unsafe.Pointer(emptyNode)

	return &FIFO{
		head: unsafe.Pointer(dummyNode),
		tail: unsafe.Pointer(dummyNode),
	}
}

// Put put an data into queue's tail
func (f *FIFO) Put(d interface{}) {
	var newNode *fifoNode
	for {
		newNode = fifoPool.Get().(*fifoNode)
		if newNode.Refcnt() == 0 {
			break
		}
	}

	newNode.d = d
	newNode.next = unsafe.Pointer(emptyNode)
	newAddr := unsafe.Pointer(newNode)

	var tailAddr unsafe.Pointer
	for {
		tailAddr = atomic.LoadPointer(&f.tail)
		tailNode := (*fifoNode)(tailAddr)
		if atomic.CompareAndSwapPointer(&tailNode.next, unsafe.Pointer(emptyNode), newAddr) {
			atomic.AddInt64(&f.len, 1)
			break
		}

		// tail may not be the exact tail node, so we need to check again
		atomic.CompareAndSwapPointer(&f.tail, tailAddr, atomic.LoadPointer(&tailNode.next))
	}

	atomic.CompareAndSwapPointer(&f.tail, tailAddr, newAddr)
}

// Get pop data from the head of queue
func (f *FIFO) Get() interface{} {
	for {
		headAddr := atomic.LoadPointer(&f.head)
		headNode := (*fifoNode)(headAddr)
		if headNode.AddRef(1) < 0 {
			headNode.AddRef(-1)
			continue
		}

		nextAddr := atomic.LoadPointer(&headNode.next)
		if nextAddr == unsafe.Pointer(emptyNode) {
			// queue is empty
			return nil
		}

		nextNode := (*fifoNode)(nextAddr)
		if atomic.CompareAndSwapPointer(&f.head, headAddr, nextAddr) {
			// do not release refcnt
			atomic.AddInt64(&f.len, -1)
			atomic.StoreInt32(&headNode.refcnt, math.MinInt32)
			fifoPool.Put(headNode)
			return nextNode.d
		}
	}
}

// Len return the length of queue
func (f *FIFO) Len() int {
	return int(atomic.LoadInt64(&f.len))
}
