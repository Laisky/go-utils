// Package counter contains varias counter tools
package counter

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/go-utils/v3/log"
)

// Int64CounterItf counter for int64
type Int64CounterItf interface {
	Count() int64
	CountN(n int64) int64
}

// ===================================

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
		lastT: gutils.UTCNow(),
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
	r = math.Round(float64(c.Get()-c.lastN)/gutils.UTCNow().Sub(c.lastT).Seconds()*100) / 100
	c.lastT = gutils.UTCNow()
	c.lastN = c.Get()
	c.Unlock()
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

var rotateCounterChanLength = 10000

// RotateCounter rotate counter
type RotateCounter struct {
	gutils.Mutex
	rotateRunner   sync.Once
	n, rotatePoint int64
	c              chan int64
	stopChan       chan struct{}
}

// NewRotateCounter create new RotateCounter with threshold from 0
func NewRotateCounter(rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(context.Background(), 0, rotatePoint)
}

// NewRotateCounterWithCtx create new RotateCounter with threshold from 0
func NewRotateCounterWithCtx(ctx context.Context, rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(ctx, 0, rotatePoint)

}

// NewRotateCounterFromN create new RotateCounter with threshold from N
func NewRotateCounterFromN(n, rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(context.Background(), n, rotatePoint)
}

// NewRotateCounterFromNWithCtx create new RotateCounter with threshold from N
func NewRotateCounterFromNWithCtx(ctx context.Context, n, rotatePoint int64) (*RotateCounter, error) {
	if rotatePoint <= 0 {
		return nil, errors.Errorf("rotatePoint should bigger than 0, but got %d", rotatePoint)
	}
	if n < 0 {
		return nil, errors.Errorf("n should bigger than 0, but got %d", n)
	}
	if n >= rotatePoint {
		return nil, errors.Errorf("n should less than rotatePoint, got n %d, rotatePoint %d", n, rotatePoint)
	}
	c := &RotateCounter{
		n:           n,
		rotatePoint: rotatePoint,
		c:           make(chan int64, rotateCounterChanLength),
	}
	go c.runRotator(ctx)
	return c, nil
}

// Close stop rorate runner
func (c *RotateCounter) Close() {
	c.stopChan <- struct{}{}
}

// runRotator start rotator
func (c *RotateCounter) runRotator(ctx context.Context) {
	c.rotateRunner.Do(func() {
		var n int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopChan:
				return
			default:
			}

			n = atomic.AddInt64(&c.n, 1)
			if n > c.rotatePoint {
				atomic.StoreInt64(&c.n, 1)
				n = 1
			}
			c.c <- n
		}
	})
}

// Count increse and return the result
func (c *RotateCounter) Count() int64 {
	return <-c.c
}

// Get return current counter's number
func (c *RotateCounter) Get() int64 {
	return atomic.LoadInt64(&c.n)
}

// CountN increse N and return the result
func (c *RotateCounter) CountN(n int64) (r int64) {
	if n == 0 {
		return c.Get()
	}
	for n > 0 {
		r = <-c.c
		n--
	}
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
	lockID int64
	n,
	quoteStep,
	rotatePoint int64
}

// ChildParallelCounter child of ParallelCounter
type ChildParallelCounter struct {
	sync.RWMutex
	lockID  int64
	p       *ParallelCounter
	n, maxN int64
}

// NewParallelCounter get new parallel counter
func NewParallelCounter(quoteStep, rotatePoint int64) (*ParallelCounter, error) {
	log.Shared.Debug("NewParallelCounter", zap.Int64("quoteStep", quoteStep), zap.Int64("rotatePoint", rotatePoint))
	if quoteStep <= 0 {
		quoteStep = defaultQuoteStep
	}
	if rotatePoint <= 0 || quoteStep >= rotatePoint {
		return nil, errors.Errorf("rotate should greater than quoteStep and 0")
	}

	return &ParallelCounter{
		lockID:      rand.Int63(),
		n:           0,
		quoteStep:   quoteStep,
		rotatePoint: rotatePoint,
	}, nil
}

// NewParallelCounterFromN get new parallel counter
func NewParallelCounterFromN(n, quoteStep, rotatePoint int64) (*ParallelCounter, error) {
	log.Shared.Debug("NewParallelCounter", zap.Int64("quoteStep", quoteStep), zap.Int64("rotatePoint", rotatePoint))
	if quoteStep <= 0 {
		quoteStep = defaultQuoteStep
	}
	if n < 0 {
		return nil, errors.Errorf("n must greater than 0")
	}
	if rotatePoint <= 0 || quoteStep >= rotatePoint {
		return nil, errors.Errorf("rotate should greater than quoteStep and 0")
	}

	return &ParallelCounter{
		lockID:      rand.Int63(),
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
	from = atomic.LoadInt64(&c.n)
	to = atomic.AddInt64(&c.n, step) - 1
	if c.rotatePoint > 0 && to > c.rotatePoint { // need rotate
		from, to = 0, step
		atomic.StoreInt64(&c.n, to+1)
	}
	c.Unlock()

	log.Shared.Debug("get quote",
		zap.Int64("step", step),
		zap.Int64("from", from),
		zap.Int64("to", to))
	return
}

// GetChild create new child
func (c *ParallelCounter) GetChild() *ChildParallelCounter {
	cc := &ChildParallelCounter{
		lockID: rand.Int63(),
		p:      c,
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
	c.RLock()
	r = atomic.AddInt64(&c.n, 1)
	cmax := atomic.LoadInt64(&c.maxN)
	c.RUnlock()
	if r > cmax {
		// log.Shared.Info("try acquire child lock", zap.Int64("r", r), zap.Int64("lid", c.lockID))
		c.Lock()
		// log.Shared.Info("acquired child lock", zap.Int64("r", r), zap.Int64("lid", c.lockID))

		// double check
		r = atomic.AddInt64(&c.n, 1) % c.p.rotatePoint
		cmax = atomic.LoadInt64(&c.maxN)
		if r > cmax {
			r, cmax = c.p.GetQuote(0)
		}
		atomic.StoreInt64(&c.n, r)
		atomic.StoreInt64(&c.maxN, cmax)

		// fmt.Println(">>", r, cmax)
		// log.Shared.Info("release child lock", zap.Int64("r", r), zap.Int64("lid", c.lockID), zap.Int64("to", cmax))
		c.Unlock()
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
