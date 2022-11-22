// Package algorithm contains some useful algorithms
package algorithm

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Laisky/errors"
	skiplist "github.com/Laisky/fast-skiplist"
	"github.com/Laisky/zap"
	"github.com/gammazero/deque"

	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/go-utils/v3/log"
)

// -------------------------------------
// deque
// -------------------------------------

// Deque
//
// https://pkg.go.dev/github.com/gammazero/deque#Deque
type Deque[T any] interface {
	PushBack(T)
	PushFront(T)
	PopFront() T
	PopBack() T
	Len() int
	Front() T
	Back() T
}

type dequeOpt struct {
	currentCapacity,
	minimalCapacity int
}

func (o *dequeOpt) applyFuncs(optfs ...DequeOptFunc) (*dequeOpt, error) {
	for _, optf := range optfs {
		if err := optf(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// DequeOptFunc optional arguments for deque
type DequeOptFunc func(*dequeOpt) error

// WithDequeCurrentCapacity preallocate memory for deque
func WithDequeCurrentCapacity(size int) DequeOptFunc {
	return func(opt *dequeOpt) error {
		if size < 0 {
			return errors.Errorf("size must greater than 0")
		}

		opt.currentCapacity = size
		return nil
	}
}

// WithDequeMinimalCapacity set deque minimal capacity
func WithDequeMinimalCapacity(size int) DequeOptFunc {
	return func(opt *dequeOpt) error {
		if size < 0 {
			return errors.Errorf("size must greater than 0")
		}

		opt.minimalCapacity = size
		return nil
	}
}

// NewDeque new deque
func NewDeque[T any](optfs ...DequeOptFunc) (Deque[T], error) {
	opt, err := new(dequeOpt).applyFuncs(optfs...)
	if err != nil {
		return nil, err
	}

	return deque.New[T](opt.currentCapacity, opt.minimalCapacity), nil
}

// -------------------------------------
// skiplist
// -------------------------------------

// NewSkiplist new skiplist
//
// https://github.com/sean-public/fast-skiplist
func NewSkiplist() *skiplist.SkipList {
	return skiplist.New()
}

// -------------------------------------
// Heap
// -------------------------------------

// itemType item that need to sort
type itemType[T gutils.Sortable] struct {
	priority T
	key      any
}

// GetKey get key of item
func (it *itemType[T]) GetKey() any {
	return it.key
}

// GetPriority get priority of item
func (it *itemType[T]) GetPriority() T {
	return it.priority
}

// HeapSlice slice that could be used by heap
type HeapSlice[T gutils.Sortable] []HeapItemItf[T]

// innerHeapQ lower structure used by heap
//
// do not use this structure directly
type innerHeapQ[T gutils.Sortable] struct {
	isMaxTop bool
	q        []HeapItemItf[T]
}

// newInnerHeapQ create new PriorityQ
func newInnerHeapQ[T gutils.Sortable](isMaxTop bool) *innerHeapQ[T] {
	return &innerHeapQ[T]{
		isMaxTop: isMaxTop,
		q:        []HeapItemItf[T]{},
	}
}

// Len get length of items in heapq
func (p *innerHeapQ[T]) Len() int {
	return len(p.q)
}

// Less compare two items in heapq
func (p *innerHeapQ[T]) Less(i, j int) bool {
	if p.isMaxTop {
		return p.q[i].GetPriority() < p.q[j].GetPriority()
	}

	return p.q[i].GetPriority() >= p.q[j].GetPriority()
}

// Swap swat two items in heapq
func (p *innerHeapQ[T]) Swap(i, j int) {
	p.q[i], p.q[j] = p.q[j], p.q[i]
}

// Push push new item into heapq
func (p *innerHeapQ[T]) Push(x any) {
	item := x.(HeapItemItf[T])
	p.q = append(p.q, item)
}

// Remove remove an specific item
func (p *innerHeapQ[T]) Remove(key any) (ok bool) {
	for i, it := range p.q {
		if it.GetKey() == key {
			p.q = append(p.q[:i], p.q[i+1:]...)
			return true
		}
	}

	return false
}

// Get get item by key
func (p *innerHeapQ[T]) Get(key any) HeapItemItf[T] {
	for i := range p.q {
		if p.q[i].GetKey() == key {
			return p.q[i]
		}
	}

	return nil
}

// GetIdx get item by idx
func (p *innerHeapQ[T]) GetIdx(idx int) HeapItemItf[T] {
	return p.q[idx]
}

// Pop pop from the tail.
// if `isMaxTop=True`, pop the tail(smallest) item
func (p *innerHeapQ[T]) Pop() (popped any) {
	n := len(p.q)
	if n == 0 {
		return nil
	}

	popped = p.q[n-1]
	p.q[n-1] = nil // avoid memory leak
	p.q = p.q[:n-1]
	return popped
}

// HeapItemItf items need to sort
//
// T is the type of priority
type HeapItemItf[T gutils.Sortable] interface {
	GetKey() any
	GetPriority() T
}

// GetLargestNItems get N highest priority items
func GetLargestNItems[T gutils.Sortable](inputChan <-chan HeapItemItf[T], topN int) ([]HeapItemItf[T], error) {
	return GetTopKItems(inputChan, topN, false)
}

// GetSmallestNItems get N smallest priority items
func GetSmallestNItems[T gutils.Sortable](inputChan <-chan HeapItemItf[T], topN int) ([]HeapItemItf[T], error) {
	return GetTopKItems(inputChan, topN, true)
}

// GetTopKItems calculate topN by heap
//
// Arg isHighest:
//   - use min-heap to calculates topN Highest items.
//   - use max-heap to calculates topN Lowest items.
func GetTopKItems[T gutils.Sortable](
	inputChan <-chan HeapItemItf[T],
	topN int,
	isHighest bool,
) ([]HeapItemItf[T], error) {
	log.Shared.Debug("GetMostFreqWords for key2PriMap", zap.Int("topN", topN))
	if topN < 2 {
		return nil, errors.Errorf("GetMostFreqWords topN must larger than 2")
	}

	var (
		i               int
		ok              bool
		item, thresItem HeapItemItf[T]
		items           = make([]HeapItemItf[T], topN)
		nTotal          = 0
		p               = newInnerHeapQ[T](isHighest)
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

		p.Push(&itemType[T]{
			key:      item.GetKey(),
			priority: item.GetPriority(),
		})
	}

	if inputChan == nil {
		switch p.Len() {
		case 1: // only one item
			return []HeapItemItf[T]{item}, nil
		case 0:
			return []HeapItemItf[T]{}, nil
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

			heap.Push(p, &itemType[T]{
				priority: item.GetPriority(),
				key:      item.GetKey(),
			})
			thresItem = heap.Pop(p).(*itemType[T])
		}
	}

	log.Shared.Debug("process all items", zap.Int("total", nTotal))
	for i := 1; i <= topN; i++ { // pop all needed items
		item = heap.Pop(p).(*itemType[T])
		items[topN-i] = item
	}

	return items, nil
}

// LimitSizeHeap heap that with limited size
type LimitSizeHeap[T gutils.Sortable] interface {
	Push(item HeapItemItf[T]) HeapItemItf[T]
	Pop() HeapItemItf[T]
}

// limitSizeHeap heap with limit size
type limitSizeHeap[T gutils.Sortable] struct {
	q             *innerHeapQ[T]
	thresItem     HeapItemItf[T]
	isHighest     bool
	size, maxSize int64
}

// NewLimitSizeHeap create new LimitSizeHeap
func NewLimitSizeHeap[T gutils.Sortable](size int, isHighest bool) (LimitSizeHeap[T], error) {
	if size < 1 {
		return nil, errors.Errorf("size must greater than 0")
	}

	h := &limitSizeHeap[T]{
		q:         newInnerHeapQ[T](!isHighest),
		maxSize:   int64(size),
		isHighest: isHighest,
	}
	heap.Init(h.q)
	return h, nil
}

// Push push item into heap, return popped item if exceed size
func (h *limitSizeHeap[T]) Push(item HeapItemItf[T]) HeapItemItf[T] {
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
		h.thresItem = heap.Pop(h.q).(HeapItemItf[T])
		return h.thresItem
	}

	return nil
}

// Pop pop from the tail.
// if `isHighest=True`, pop the biggest item
func (h *limitSizeHeap[T]) Pop() HeapItemItf[T] {
	if h.size == 0 {
		return nil
	}

	h.size--
	return heap.Pop(h.q).(HeapItemItf[T])
}

// -------------------------------------
// FIFO
// -------------------------------------

var fifoPool = sync.Pool{
	New: func() any {
		return &fifoNode{
			next: unsafe.Pointer(emptyNode),
		}
	},
}

type fifoNode struct {
	next unsafe.Pointer
	d    any
	// refcnt to avoid ABA problem
	// refcnt int32
}

// CompareAndAdd add ref count
// func (f *fifoNode) CompareAndAdd(expect int32) bool {
// 	return atomic.CompareAndSwapInt32(&f.refcnt, expect, expect+1)
// }

// Refcnt get ref count
// func (f *fifoNode) Refcnt() int32 {
// 	return atomic.LoadInt32(&f.refcnt)
// }

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
	tail  unsafe.Pointer
	len   int64
	dummy unsafe.Pointer
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
		head:  unsafe.Pointer(dummyNode),
		tail:  unsafe.Pointer(dummyNode),
		dummy: unsafe.Pointer(dummyNode),
	}
}

// Put put an data into queue's tail
func (f *FIFO) Put(d any) {
	newNode := fifoPool.Get().(*fifoNode)
	// for {
	// 	newNode = fifoPool.Get().(*fifoNode)
	// 	if newNode.AddRef(1) == 1 {
	// 		break
	// 	}

	// 	runtime.Gosched()
	// 	continue
	// }

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
func (f *FIFO) Get() any {
	for {
		headAddr := atomic.LoadPointer(&f.head)
		headNode := (*fifoNode)(headAddr)
		// if !headNode.CompareAndAdd(1) {
		// 	// someone already get this node from pool
		// 	runtime.Gosched()
		// 	continue
		// }

		nextAddr := atomic.LoadPointer(&headNode.next)
		if nextAddr == unsafe.Pointer(emptyNode) {
			// queue is empty
			return nil
		}

		nextNode := (*fifoNode)(nextAddr)
		if atomic.CompareAndSwapPointer(&f.head, headAddr, nextAddr) {
			// do not release refcnt
			atomic.AddInt64(&f.len, -1)
			// atomic.StoreInt32(&headNode.refcnt, 0)
			// fifoPool.Put(headNode)
			return nextNode.d
		}
	}
}

// Len return the length of queue
func (f *FIFO) Len() int {
	return int(atomic.LoadInt64(&f.len))
}
