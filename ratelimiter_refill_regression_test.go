package utils

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeRefillClock is a manually advanced clock for refill-loop tests.
type fakeRefillClock struct {
	mu  sync.Mutex
	now time.Time
}

// Now returns the current fake time.
func (c *fakeRefillClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the fake clock forward by d.
func (c *fakeRefillClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newRefillTestLimiter builds a limiter whose state is initialized with zero
// tokens but whose refill loop is not started, so the test can drive it.
func newRefillTestLimiter(t *testing.T, args RateLimiterArgs) *RateLimiter {
	t.Helper()
	manager := NewMemoryRateLimiterStateManager()
	_, err := manager.Setup(context.Background(), args, 0)
	require.NoError(t, err)
	return &RateLimiter{
		RateLimiterArgs: args,
		ctx:             context.Background(),
		stateCtx:        context.Background(),
		stateManager:    manager,
		stopChan:        make(chan struct{}),
	}
}

// waitForTokens polls the limiter until it reports want tokens or fails.
func waitForTokens(t *testing.T, rl *RateLimiter, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return rl.Len() == want },
		2*time.Second, time.Millisecond, "want %d tokens, got %d", want, rl.Len())
}

// TestRateLimiterRefillTracksElapsedTimeWhenTicksAreDropped verifies that the
// refill amount follows elapsed time rather than the number of ticks received.
// time.Ticker drops ticks for a slow receiver, so a tick-counting refill
// under-delivers the configured rate whenever the refill goroutine is delayed
// (for example on a loaded host).
func TestRateLimiterRefillTracksElapsedTimeWhenTicksAreDropped(t *testing.T) {
	t.Parallel()
	rl := newRefillTestLimiter(t, RateLimiterArgs{NPerSec: 10, Max: 100})
	clock := &fakeRefillClock{now: time.Unix(1_700_000_000, 0)}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		rl.refillLoop(ctx, ticks, clock.Now)
	}()

	// The first tick only synchronizes with the loop after it read the clock.
	ticks <- clock.Now()

	// One delayed tick after a full second must refill a full second of tokens.
	clock.Advance(time.Second)
	ticks <- clock.Now()
	waitForTokens(t, rl, 10)

	// A long stall must never refill more than Max.
	clock.Advance(time.Hour)
	ticks <- clock.Now()
	waitForTokens(t, rl, 100)

	cancel()
	<-done
}

// TestRateLimiterRefillAccumulatesFractionalTokens verifies that sub-token
// refills carry over between ticks instead of being lost.
func TestRateLimiterRefillAccumulatesFractionalTokens(t *testing.T) {
	t.Parallel()
	rl := newRefillTestLimiter(t, RateLimiterArgs{NPerSec: 3, Max: 30})
	clock := &fakeRefillClock{now: time.Unix(1_700_000_000, 0)}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go rl.refillLoop(ctx, ticks, clock.Now)

	// The first tick only synchronizes with the loop after it read the clock.
	ticks <- clock.Now()
	for range 10 {
		clock.Advance(100 * time.Millisecond)
		ticks <- clock.Now()
	}
	waitForTokens(t, rl, 3)
}

// manualRefill drives the refill loop of a RateLimiter with a fake clock and an unbuffered tick channel, so a
// test decides exactly when and by how much the limiter refills, independent of host load.
type manualRefill struct {
	clock *fakeRefillClock
	ticks chan time.Time
}

// newManuallyRefilledLimiter creates a RateLimiter through NewRateLimiter whose refill loop is driven by the
// returned manualRefill instead of a real ticker. It takes the test handle, the limiter args and the initial
// token count, and returns the limiter and its driver. The state manager is initialized before NewRateLimiter
// runs, so NewRateLimiter starts no background refill of its own; the cleanup stops the driven loop and closes
// the limiter, so callers must not call Close themselves.
func newManuallyRefilledLimiter(t *testing.T, args RateLimiterArgs, initialTokens int) (*RateLimiter, *manualRefill) {
	t.Helper()
	manager := NewMemoryRateLimiterStateManager()
	shouldRefill, err := manager.Setup(context.Background(), args, initialTokens)
	require.NoError(t, err)
	require.True(t, shouldRefill)

	rl, err := NewRateLimiter(context.Background(), args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)

	driver := &manualRefill{
		clock: &fakeRefillClock{now: time.Unix(1_700_000_000, 0)},
		ticks: make(chan time.Time),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		rl.refillLoop(ctx, driver.ticks, driver.clock.Now)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		rl.Close()
	})

	// The first tick is received only after the loop read its start time, and refills nothing.
	driver.ticks <- driver.clock.Now()
	return rl, driver
}

// advance moves the fake clock forward by d and delivers one tick for it. It then delivers a second tick
// without advancing the clock, which refills nothing and which the loop can receive only after it finished
// applying the first one, so the refill is complete when advance returns. It returns nothing.
func (m *manualRefill) advance(d time.Duration) {
	m.clock.Advance(d)
	m.ticks <- m.clock.Now()
	m.ticks <- m.clock.Now()
}
