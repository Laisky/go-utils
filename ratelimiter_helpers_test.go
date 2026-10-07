package utils

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// noRefillStateManager wraps a MemoryRateLimiterStateManager and always tells NewRateLimiter not to start a
// refill loop. Token counts of a limiter built on it change only through the test's own calls, so exact counts
// cannot be disturbed by a background refill that runs while a loaded host deschedules the test.
type noRefillStateManager struct {
	*MemoryRateLimiterStateManager
}

// Setup initializes the wrapped manager with args and initialTokens through the given ctx. It returns false, so
// the calling limiter starts no refill loop, and the wrapped manager's error, if any.
func (m noRefillStateManager) Setup(ctx context.Context, args RateLimiterArgs, initialTokens int) (bool, error) {
	_, err := m.MemoryRateLimiterStateManager.Setup(ctx, args, initialTokens)
	return false, err
}

// newNoRefillLimiter creates a RateLimiter through NewRateLimiter with the given args and options, backed by a
// noRefillStateManager so that it never refills on its own. Options such as WithAvailableTokens and
// WithRateLimiterState still decide the initial balance. It takes the test handle, the args and the options,
// returns the limiter, and closes it when the test ends, so callers must not call Close themselves.
func newNoRefillLimiter(t *testing.T, args RateLimiterArgs, opts ...RateLimiterOption) *RateLimiter {
	t.Helper()
	manager := noRefillStateManager{NewMemoryRateLimiterStateManager()}
	opts = append(opts, WithRateLimiterStateManager(manager))
	rl, err := NewRateLimiter(context.Background(), args, opts...)
	require.NoError(t, err)
	t.Cleanup(rl.Close)
	return rl
}

// manualRefill drives the refill loop of a RateLimiter with a fake clock and an unbuffered tick channel, so a
// test decides exactly when and by how much the limiter refills, independent of host load.
type manualRefill struct {
	clock *fakeRefillClock
	ticks chan time.Time
}

// newManuallyRefilledLimiter creates a RateLimiter like newNoRefillLimiter and runs its refill loop driven by the
// returned manualRefill instead of a real ticker. It takes the test handle, the limiter args and options, and
// returns the limiter and its driver. The test cleanup stops the driven loop before the limiter is closed.
func newManuallyRefilledLimiter(t *testing.T, args RateLimiterArgs, opts ...RateLimiterOption) (*RateLimiter, *manualRefill) {
	t.Helper()
	rl := newNoRefillLimiter(t, args, opts...)
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
	t.Cleanup(func() { // runs before the limiter's Close, which was registered earlier
		cancel()
		<-done
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

// requireWithinRefillRate checks the token-bucket invariant of a real, background-refilled limiter: the tokens it
// handed out plus the tokens it still holds never exceed its initial tokens plus NPerSec times the time measured
// since start (plus one for rounding), however the host scheduled the refill goroutine. It takes the test handle,
// the limiter, its initial balance, the number of tokens consumed so far and the time taken before the limiter
// was created, and returns nothing.
func requireWithinRefillRate(t *testing.T, rl *RateLimiter, initial, consumed int, start time.Time) {
	t.Helper()
	held := rl.Len() // read before measuring time, so the bound covers it
	ceiling := float64(initial) + float64(rl.NPerSec)*time.Since(start).Seconds() + 1
	require.LessOrEqual(t, float64(consumed+held), ceiling,
		"consumed %d + held %d tokens exceed %d initial + %d/s over the measured time", consumed, held, initial,
		rl.NPerSec)
}

// awaitTokens waits until rl holds at least want tokens. The refill goroutine may be starved on a loaded host,
// so the wait is bounded generously instead of by the nominal refill time. It takes the test handle, the
// limiter and the wanted balance, and returns nothing.
func awaitTokens(t *testing.T, rl *RateLimiter, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return rl.Len() >= want }, 30*time.Second, 5*time.Millisecond,
		"limiter never reached %d tokens, holds %d", want, rl.Len())
}

// consumeUntil calls Allow every millisecond until it has admitted want requests, and returns how long that took
// from start. It takes the test handle, the limiter, the number of requests to admit and the start time, and
// fails the test when the requests are not admitted within a generous deadline.
func consumeUntil(t *testing.T, rl *RateLimiter, want int, start time.Time) time.Duration {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for allowed := 0; allowed < want; {
		if rl.Allow() {
			allowed++
			continue
		}
		require.True(t, time.Now().Before(deadline), "only %d of %d requests admitted before the deadline",
			allowed, want)
		time.Sleep(time.Millisecond)
	}
	return time.Since(start)
}

// countingStateManager wraps a MemoryRateLimiterStateManager and records how many Setup calls asked for a refill
// loop and how many tokens refill loops requested in total, before the Max cap is applied.
type countingStateManager struct {
	*MemoryRateLimiterStateManager
	mu        sync.Mutex
	refillers int
	requested int
}

// Setup delegates to the wrapped manager with ctx, args and initialTokens, counts the calls that start a refill
// loop, and returns the wrapped manager's result.
func (m *countingStateManager) Setup(ctx context.Context, args RateLimiterArgs, initialTokens int) (bool, error) {
	shouldRefill, err := m.MemoryRateLimiterStateManager.Setup(ctx, args, initialTokens)
	if shouldRefill {
		m.mu.Lock()
		m.refillers++
		m.mu.Unlock()
	}
	return shouldRefill, err
}

// AddTokens records the n tokens a refill loop requests through ctx and delegates to the wrapped manager. It
// returns the number of tokens actually added and the wrapped manager's error.
func (m *countingStateManager) AddTokens(ctx context.Context, n int) (int, error) {
	m.mu.Lock()
	m.requested += n
	m.mu.Unlock()
	return m.MemoryRateLimiterStateManager.AddTokens(ctx, n)
}

// counts returns the number of refill loops started and the total number of tokens they requested so far.
func (m *countingStateManager) counts() (refillers, requested int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refillers, m.requested
}
