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
