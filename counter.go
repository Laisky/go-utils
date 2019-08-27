package utils

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/zap"
)

// Counter int64 counter
type Counter struct {
	sync.Mutex
	n, lastN int64
	lastT    time.Time
}

// NewCounter create Counter from 0
func NewCounter() *Counter {
	return &Counter{
		n:     0,
		lastT: UTCNow(),
		lastN: 0,
	}
}

// NewCounterFromN create Counter from custom number
func NewCounterFromN(n int64) *Counter {
	return &Counter{
		n: n,
	}
}

// Get return current counter's number
func (c *Counter) Get() int64 {
	return atomic.LoadInt64(&c.n)
}

// GetSpeed return increasing speed from lastest invoke `GetSpeed`
func (c *Counter) GetSpeed() (r float64) {
	c.Lock()
	defer c.Unlock()

	r = Round(float64(c.Get()-c.lastN)/UTCNow().Sub(c.lastT).Seconds(), .5, 2)
	c.lastT = UTCNow()
	c.lastN = c.Get()
	return r
}

// Set overwrite the counter's number
func (c *Counter) Set(n int64) {
	atomic.StoreInt64(&c.n, n)
}

// Count increse and return the result
func (c *Counter) Count() int64 {
	return atomic.AddInt64(&c.n, 1)
}

// CountN increse N and return the result
func (c *Counter) CountN(n int64) int64 {
	return atomic.AddInt64(&c.n, n)
}

// -------------------------------------------------

var rotateCounterChanLength = 1000

// RotateCounter rotate counter
type RotateCounter struct {
	n, rotatePoint int64
	c              chan int64
}

// NewRotateCounter create new RotateCounter with threshold from 0
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

// NewRotateCounterFromN create new RotateCounter with threshold from N
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

// Count increse and return the result
func (c *RotateCounter) Count() int64 {
	return <-c.c
}

// CountN increse N and return the result
func (c *RotateCounter) CountN(n int64) (r int64) {
	for i := int64(0); i < n-1; i++ {
		<-c.c
	}
	return <-c.c
}

// --------------------------------------------

var monotonicCounterChanLength = 10000

// MonotonicRotateCounter monotonic increse counter uncontinuity,
// has much better performance than RotateCounter.
type MonotonicRotateCounter struct {
	n, rotatePoint    int64
	innerStep, innerN int64
	c                 chan int64
}

// NewMonotonicRotateCounter return new MonotonicRotateCounter with threshold from 0
func NewMonotonicRotateCounter(rotatePoint int64) (*MonotonicRotateCounter, error) {
	if rotatePoint <= 0 {
		return nil, fmt.Errorf("rotatePoint should bigger than 0, but got %v", rotatePoint)
	}

	c := &MonotonicRotateCounter{
		rotatePoint: rotatePoint,
		c:           make(chan int64, monotonicCounterChanLength),
		innerStep:   int64(math.Max(1, math.Min(100, float64(rotatePoint)/10))),
	}
	Logger.Debug("set inner step", zap.Int64("inner_step", c.innerStep))
	go c.runGenerator()
	return c, nil
}

// NewMonotonicCounterFromN return new MonotonicRotateCounter with threshold from n
func NewMonotonicCounterFromN(n, rotatePoint int64) (*MonotonicRotateCounter, error) {
	if rotatePoint <= 0 {
		return nil, fmt.Errorf("rotatePoint should bigger than 0, but got %v", rotatePoint)
	}
	if n < 0 {
		return nil, fmt.Errorf("n should bigger than 0, but got %v", n)
	}
	if n >= rotatePoint {
		return nil, fmt.Errorf("n should less than rotatePoint, got n %v, rotatePoint %v", n, rotatePoint)
	}

	c := &MonotonicRotateCounter{
		n:           n,
		rotatePoint: rotatePoint,
		c:           make(chan int64, monotonicCounterChanLength),
		innerStep:   int64(math.Max(1, math.Min(100, float64(rotatePoint)/10))),
	}
	c.rotatePoint -= c.innerN // `Count` at most add  `c.innerN`
	Logger.Debug("set inner step", zap.Int64("inner_step", c.innerStep))
	go c.runGenerator()
	return c, nil
}

func (c *MonotonicRotateCounter) runGenerator() {
	c.n += c.innerStep
	for {
		c.c <- c.n
		c.n += c.innerStep
		if c.n >= c.rotatePoint {
			c.n = 0
		}
	}
}

// Count increse and return the result
func (c *MonotonicRotateCounter) Count() (n int64) {
	if atomic.LoadInt64(&c.innerN)%c.innerStep == c.innerStep-1 {
		n = <-c.c
		atomic.StoreInt64(&c.innerN, n)
		return n
	}

	return atomic.AddInt64(&c.innerN, 1)
}

// CountN increse N and return the result
func (c *MonotonicRotateCounter) CountN(n int64) (r int64) {
	for i := 0; i < FloorDivision(int(n), int(c.innerStep)); i++ {
		<-c.c
	}

	r = <-c.c
	atomic.StoreInt64(&c.innerN, r)
	return r
}

// ---------------------------------------------------

// Uint32Counter uint32 counter
type Uint32Counter struct {
	n uint32
}

// NewUint32Counter return new Uint32Counter from 0
func NewUint32Counter() *Uint32Counter {
	return &Uint32Counter{
		n: 0,
	}
}

// NewUint32CounterFromN return new Uint32Counter from n
func NewUint32CounterFromN(n uint32) *Uint32Counter {
	return &Uint32Counter{
		n: n,
	}
}

// Get return current counter's number
func (c *Uint32Counter) Get() uint32 {
	return atomic.LoadUint32(&c.n)
}

// Set overwrite the counter's number
func (c *Uint32Counter) Set(n uint32) {
	atomic.StoreUint32(&c.n, n)
}

// Count increse and return the result
func (c *Uint32Counter) Count() uint32 {
	return atomic.AddUint32(&c.n, 1)
}

// CountN increse N and return the result
func (c *Uint32Counter) CountN(n uint32) uint32 {
	return atomic.AddUint32(&c.n, n)
}

// ---------------------------------------------------

const defaultQuoteStep = 1000

// ParallelCounter parallel count with child counter
type ParallelCounter struct {
	sync.Mutex
	n,
	quoteStep,
	rotatePoint int64
}

// ChildParallelCounter child of ParallelCounter
type ChildParallelCounter struct {
	sync.Mutex
	p       *ParallelCounter
	n, maxN int64
}

// NewParallelCounter get new parallel counter
func NewParallelCounter(quoteStep, rotatePoint int64) (*ParallelCounter, error) {
	Logger.Debug("NewParallelCounter", zap.Int64("quoteStep", quoteStep), zap.Int64("rotatePoint", rotatePoint))
	if quoteStep <= 0 {
		quoteStep = defaultQuoteStep
	}
	if rotatePoint <= 0 || quoteStep >= rotatePoint {
		return nil, fmt.Errorf("rotate should greater than quoteStep and 0")
	}

	return &ParallelCounter{
		n:           0,
		quoteStep:   quoteStep,
		rotatePoint: rotatePoint,
	}, nil
}

// NewParallelCounterFromN get new parallel counter
func NewParallelCounterFromN(n, quoteStep, rotatePoint int64) (*ParallelCounter, error) {
	Logger.Debug("NewParallelCounter", zap.Int64("quoteStep", quoteStep), zap.Int64("rotatePoint", rotatePoint))
	if quoteStep <= 0 {
		quoteStep = defaultQuoteStep
	}
	if n < 0 {
		return nil, fmt.Errorf("n must greater than 0")
	}
	if rotatePoint <= 0 || quoteStep >= rotatePoint {
		return nil, fmt.Errorf("rotate should greater than quoteStep and 0")
	}

	return &ParallelCounter{
		n:           n,
		quoteStep:   quoteStep,
		rotatePoint: rotatePoint,
	}, nil
}

// GetQuote child request new quote from parent
func (c *ParallelCounter) GetQuote(step int64) (from, to int64) {
	if step <= 0 {
		step = c.quoteStep
	}
	if c.rotatePoint > 0 && step > c.rotatePoint {
		step = step % c.rotatePoint
	}

	c.Lock()
	defer c.Unlock()

	from = atomic.LoadInt64(&c.n)
	to = atomic.AddInt64(&c.n, step) - 1
	if c.rotatePoint > 0 && to > c.rotatePoint { // need rotate
		from = 0
		to = step
	}

	Logger.Debug("get quote",
		zap.Int64("step", step),
		zap.Int64("from", from),
		zap.Int64("to", to))
	return
}

// GetChild create new child
func (c *ParallelCounter) GetChild() *ChildParallelCounter {
	cc := &ChildParallelCounter{
		p: c,
	}
	cc.n, cc.maxN = c.GetQuote(c.quoteStep)
	return cc
}

// Get get current count
func (c *ChildParallelCounter) Get() int64 {
	return atomic.LoadInt64(&c.n)
}

// Count count 1
func (c *ChildParallelCounter) Count() (r int64) {
	r = atomic.AddInt64(&c.n, 1)
	if r > atomic.LoadInt64(&c.maxN) {
		c.Lock()
		defer c.Unlock()
		if r > c.p.rotatePoint {
			r = r % c.p.rotatePoint
		}
		var (
			cn   = atomic.LoadInt64(&c.n)
			cmax = atomic.LoadInt64(&c.maxN)
		)
		if r < cmax && r >= cn { // already get new quote
			cn = atomic.AddInt64(&c.n, 1)
			r = cn
		}

		for r > cmax { // double check
			cn, cmax = c.p.GetQuote(0)
			r = cn
		}

		atomic.StoreInt64(&c.n, cn)
		atomic.StoreInt64(&c.maxN, cmax)
	}

	return r
}

// CountN count n
func (c *ChildParallelCounter) CountN(n int64) (r int64) {
	for i := int64(0); i < n-1; i++ {
		c.Count()
	}

	return c.Count()
}
