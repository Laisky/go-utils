package utils

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Counter struct {
	*sync.Mutex
	n, lastN int64
	lastT    time.Time
}

func NewCounter() *Counter {
	return &Counter{
		n:     0,
		lastT: time.Now(),
		lastN: 0,
	}
}

func NewCounterFromN(n int64) *Counter {
	return &Counter{
		n: n,
	}
}

func (c *Counter) Get() int64 {
	return atomic.LoadInt64(&c.n)
}

func (c *Counter) GetSpeed() (r float64) {
	c.Lock()
	defer c.Unlock()

	r = Round(float64(c.Get()-c.lastN)/time.Now().Sub(c.lastT).Seconds(), .5, 2)
	c.lastT = time.Now()
	c.lastN = c.Get()
	return r
}

func (c *Counter) Set(n int64) {
	atomic.StoreInt64(&c.n, n)
}

func (c *Counter) Count() int64 {
	return atomic.AddInt64(&c.n, 1)
}

func (c *Counter) CountN(n int64) int64 {
	return atomic.AddInt64(&c.n, n)
}

// -------------------------------------------------

var rotateCounterChanLength = 1000

type RotateCounter struct {
	n, rotatePoint int64
	c              chan int64
}

func NewRotateCounter(rotatePoint int64) (*RotateCounter, error) {
	if rotatePoint <= 0 {
		return nil, fmt.Errorf("rotatePoint should bigger than 0, but got %v", rotatePoint)
	}
	c := &RotateCounter{
		rotatePoint: rotatePoint,
		c:           make(chan int64, rotateCounterChanLength),
	}
	go c.runGenerator()
	return c, nil
}

func NewRotateCounterFromN(n, rotatePoint int64) (*RotateCounter, error) {
	if rotatePoint <= 0 {
		return nil, fmt.Errorf("rotatePoint should bigger than 0, but got %v", rotatePoint)
	}
	if n < 0 {
		return nil, fmt.Errorf("n should bigger than 0, but got %v", n)
	}
	if n >= rotatePoint {
		return nil, fmt.Errorf("n should less than rotatePoint, got n %v, rotatePoint %v", n, rotatePoint)
	}
	c := &RotateCounter{
		n:           n,
		rotatePoint: rotatePoint,
		c:           make(chan int64, rotateCounterChanLength),
	}
	go c.runGenerator()
	return c, nil
}

func (c *RotateCounter) runGenerator() {
	for {
		c.c <- c.n
		c.n++
		if c.n == c.rotatePoint {
			c.n = 0
		}
	}
}

func (c *RotateCounter) Count() int64 {
	return <-c.c
}

func (c *RotateCounter) CountN(n int64) (r int64) {
	for i := int64(0); i < n-1; i++ {
		<-c.c
	}
	return <-c.c
}

// ---------------------------------------------------

type Uint32Counter struct {
	n uint32
}

func NewUint32Counter() *Uint32Counter {
	return &Uint32Counter{
		n: 0,
	}
}

func NewUint32CounterFromN(n uint32) *Uint32Counter {
	return &Uint32Counter{
		n: n,
	}
}

func (c *Uint32Counter) Get() uint32 {
	return atomic.LoadUint32(&c.n)
}

func (c *Uint32Counter) Set(n uint32) {
	atomic.StoreUint32(&c.n, n)
}

func (c *Uint32Counter) Count() uint32 {
	return atomic.AddUint32(&c.n, 1)
}

func (c *Uint32Counter) CountN(n uint32) uint32 {
	return atomic.AddUint32(&c.n, n)
}
