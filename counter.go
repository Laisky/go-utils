package utils

import "sync/atomic"

type Counter struct {
	n int64
}

func NewCounter() *Counter {
	return &Counter{
		n: 0,
	}
}

func NewCounterFromN(n int64) *Counter {
	return &Counter{
		n: n,
	}
}

func (c *Counter) Get() int64 {
	return c.n
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
	return c.n
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
