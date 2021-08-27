package utils

import (
	"sync/atomic"
	"unsafe"
)

type fifoNode struct {
	next unsafe.Pointer
	d    interface{}
}

// FIFO is a lock-free First-In-First-Out queue
//
// paper: https://1drv.ms/b/s!Au45o0W1gVVLuNxYkPzfBo4fOssFPQ?e=TYxHKl
type FIFO struct {
	head unsafe.Pointer
	tail unsafe.Pointer
	len  int64
}

// NewFIFO create a new FIFO queue
func NewFIFO() *FIFO {
	// add a dummy node to the queue to avoid contention betweet head & tail
	// when queue is empty
	dummyNonde := &fifoNode{
		d: "laisky",
	}
	return &FIFO{
		head: unsafe.Pointer(dummyNonde),
		tail: unsafe.Pointer(dummyNonde),
	}
}

// Put put an data into queue's tail
func (f *FIFO) Put(d interface{}) {
	newNode := &fifoNode{
		d: d,
	}
	newAddr := unsafe.Pointer(newNode)

	var tailAddr unsafe.Pointer
	for {
		tailAddr = atomic.LoadPointer(&f.tail)
		tailNode := (*fifoNode)(tailAddr)
		if atomic.CompareAndSwapPointer(&tailNode.next, unsafe.Pointer(uintptr(0)), newAddr) {
			atomic.AddInt64(&f.len, 1)
			break
		}

		atomic.CompareAndSwapPointer(&f.tail, tailAddr, atomic.LoadPointer(&tailNode.next))
	}

	atomic.CompareAndSwapPointer(&f.tail, tailAddr, newAddr)
}

// Get pop data from the head of queue
func (f *FIFO) Get() interface{} {
	var nextNode *fifoNode
	for {
		headAddr := atomic.LoadPointer(&f.head)
		headNode := (*fifoNode)(headAddr)
		nextAddr := atomic.LoadPointer(&headNode.next)
		if nextAddr == unsafe.Pointer(uintptr(0)) {
			// queue is empty
			return nil
		}

		nextNode = (*fifoNode)(nextAddr)
		if atomic.CompareAndSwapPointer(&f.head, headAddr, nextAddr) {
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
