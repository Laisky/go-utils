package counter

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v6"
)

// ErrRotateCounterClosed reports an attempt to advance a closed counter.
var ErrRotateCounterClosed = errors.New("rotate counter is closed")

// RotateCounter is a synchronized counter with values in 1..rotatePoint after
// the first increment. It advances only when consumed; Get never prefetches.
// Construct it with NewRotateCounter or its variants. Do not copy it after use.
type RotateCounter struct {
	gutils.Mutex   // Retained for source compatibility; counting uses its own lock.
	mu             sync.Mutex
	n, rotatePoint int64
	ctx            context.Context
	closed         bool
}

// NewRotateCounter creates a counter at zero with a positive rotation threshold.
func NewRotateCounter(rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(context.Background(), 0, rotatePoint)
}

// NewRotateCounterWithCtx creates a counter at zero that stops advancing when ctx is canceled.
func NewRotateCounterWithCtx(ctx context.Context, rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(ctx, 0, rotatePoint)
}

// NewRotateCounterFromN creates a counter with 0 <= n < rotatePoint.
func NewRotateCounterFromN(n, rotatePoint int64) (*RotateCounter, error) {
	return NewRotateCounterFromNWithCtx(context.Background(), n, rotatePoint)
}

// NewRotateCounterFromNWithCtx validates the initial state and creates a counter.
// It owns no goroutines and does not cancel the caller's context.
func NewRotateCounterFromNWithCtx(ctx context.Context, n, rotatePoint int64) (*RotateCounter, error) {
	if ctx == nil {
		return nil, errors.New("create rotate counter: nil context")
	}
	if rotatePoint <= 0 || n < 0 || n >= rotatePoint {
		return nil, errors.New("create rotate counter: require 0 <= n < rotatePoint and rotatePoint > 0")
	}
	return &RotateCounter{n: n, rotatePoint: rotatePoint, ctx: ctx}, nil
}

// Close prevents future increments. It is idempotent and leaves Get unchanged.
func (c *RotateCounter) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// Get returns the most recently consumed value, or the initial value before consumption.
func (c *RotateCounter) Get() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// Count advances by one. It returns zero after Close or context cancellation.
// Use CountNChecked to distinguish lifecycle errors explicitly.
func (c *RotateCounter) Count() int64 { return c.CountN(1) }

// CountN advances by n in constant time and returns the last consumed value.
// A zero step reads the value, including after closure; a negative step or a
// lifecycle error returns zero without changing state, retaining the legacy signature.
func (c *RotateCounter) CountN(n int64) int64 {
	result, err := c.CountNChecked(n)
	if err != nil {
		return 0
	}
	return result
}

// CountNChecked is CountN with explicit errors for invalid steps, uninitialized
// counters, closure, and cancellation. Neither errors nor a zero step mutate state.
func (c *RotateCounter) CountNChecked(n int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rotatePoint <= 0 || c.ctx == nil {
		return 0, errors.New("advance rotate counter: uninitialized counter")
	}
	if n < 0 {
		return 0, errors.New("advance rotate counter: negative step")
	}
	if n == 0 {
		return c.n, nil
	}
	if c.closed {
		return 0, errors.WithStack(ErrRotateCounterClosed)
	}
	if err := c.ctx.Err(); err != nil {
		return 0, errors.Wrap(err, "advance rotate counter")
	}
	c.n = rotateAdd(c.n, n, c.rotatePoint)
	return c.n, nil
}

// rotateAdd returns (current + step) modulo rotatePoint, mapping a zero result
// to the inclusive upper endpoint rotatePoint. It requires rotatePoint > 0,
// 0 <= current <= rotatePoint and step > 0, which CountNChecked guarantees. The
// sum is formed from residues in [0, rotatePoint), so no intermediate value can
// overflow int64 and no signedness conversion is needed.
func rotateAdd(current, step, rotatePoint int64) int64 {
	current %= rotatePoint
	step %= rotatePoint
	// headroom is in [1, rotatePoint]. When step reaches it the sum wraps, and
	// step-headroom is the wrapped residue; otherwise current+step < rotatePoint.
	headroom := rotatePoint - current
	var next int64
	if step < headroom {
		next = current + step
	} else {
		next = step - headroom
	}
	if next == 0 {
		return rotatePoint
	}
	return next
}
