package utils

import (
	"sync/atomic"
	"unsafe"
)

type fifoNode struct {
	next *unsafe.Pointer
	d    interface{}
}

// FIFO is a lock-free First-In-First-Out queue
type FIFO struct {
	head *unsafe.Pointer
	tail *unsafe.Pointer
	len  int64
}

// NewFIFO create a new FIFO queue
func NewFIFO() *FIFO {
	var next unsafe.Pointer
	node := &fifoNode{
		next: &next,
		d:    "laisky",
	}
	head := unsafe.Pointer(node)
	tail := unsafe.Pointer(node)
	return &FIFO{
		head: &head,
		tail: &tail,
	}
}

// Put put an data into queue's tail
func (f *FIFO) Put(d interface{}) {
	var next unsafe.Pointer
	newNode := &fifoNode{
		d:    d,
		next: &next,
	}
	newAddr := unsafe.Pointer(newNode)

	var tailAddr unsafe.Pointer
	for {
		tailAddr = atomic.LoadPointer(f.tail)
		tailNode := (*fifoNode)(tailAddr)
		if atomic.CompareAndSwapPointer(tailNode.next, unsafe.Pointer(uintptr(0)), newAddr) {
			atomic.AddInt64(&f.len, 1)
			break
		}

		atomic.CompareAndSwapPointer(f.tail, tailAddr, atomic.LoadPointer(tailNode.next))
	}

	atomic.CompareAndSwapPointer(f.tail, tailAddr, newAddr)
}

// Get pop data from the head of queue
func (f *FIFO) Get() interface{} {
	var nextNode *fifoNode
	for {
		headAddr := atomic.LoadPointer(f.head)
		headNode := (*fifoNode)(headAddr)
		nextAddr := atomic.LoadPointer(headNode.next)
		if nextAddr == unsafe.Pointer(uintptr(0)) {
			// queue is empty
			return nil
		}

		nextNode = (*fifoNode)(nextAddr)
		if atomic.CompareAndSwapPointer(f.head, headAddr, nextAddr) {
			atomic.AddInt64(&f.len, -1)
			break
		}
	}

	return nextNode.d
}

// Len return the length of queue
func (f *FIFO) Len() int {
	return int(atomic.LoadInt64(&f.len))
}
