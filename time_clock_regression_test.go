package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// clockTestDeadline bounds how long a ClockT operation that must not block may take. It is far above any
// scheduling delay seen on a loaded host and far below the one-hour refresh interval the tests use, so a
// timeout can only mean the operation is waiting for the refresh loop rather than being slow.
const clockTestDeadline = 10 * time.Second

// requireReturnsWithin runs fn in a new goroutine and fails the test when fn has not returned within timeout.
// It takes the test handle, the timeout and the function under test, and returns nothing. A blocked fn is
// leaked on purpose, because a deadlocked goroutine cannot be stopped from the outside.
func requireReturnsWithin(t *testing.T, timeout time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		require.FailNow(t, "operation blocked", "%s did not return within %s", what, timeout)
	}
}

// TestClockCloseIsIdempotent verifies that calling Close twice returns instead of blocking forever on a
// refresh loop that already exited. It is a regression guard for ClockT.Close deadlocks.
func TestClockCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	c := NewClock(context.Background(), time.Millisecond)

	requireReturnsWithin(t, clockTestDeadline, "first Close", c.Close)
	requireReturnsWithin(t, clockTestDeadline, "second Close", c.Close)
}

// TestClockCloseAfterContextCancel verifies that Close returns after the clock's context was canceled, when the
// refresh loop has already stopped on its own. It is a regression guard for ClockT.Close deadlocks.
func TestClockCloseAfterContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	c := NewClock(ctx, time.Millisecond)
	cancel()

	// Give the loop time to observe the cancellation and exit before Close is called.
	require.Eventually(t, func() bool {
		before := c.GetUTCNow()
		time.Sleep(20 * time.Millisecond)
		return c.GetUTCNow().Equal(before)
	}, clockTestDeadline, time.Millisecond, "the refresh loop must stop after its context is canceled")

	requireReturnsWithin(t, clockTestDeadline, "Close after cancel", c.Close)
	stopped := c.GetUTCNow()
	time.Sleep(10 * time.Millisecond)
	require.True(t, c.GetUTCNow().Equal(stopped), "a stopped clock must not refresh")
}

// TestClockCloseDoesNotWaitForInterval verifies that Close stops a clock whose refresh interval is one hour
// without waiting for that interval to elapse, and that the cached time never changes after Close returns.
func TestClockCloseDoesNotWaitForInterval(t *testing.T) {
	t.Parallel()
	c := NewClock(context.Background(), time.Hour)

	requireReturnsWithin(t, clockTestDeadline, "Close of a one-hour clock", c.Close)

	stopped := c.GetUTCNow()
	c.SetInterval(time.Microsecond)
	time.Sleep(10 * time.Millisecond)
	require.True(t, c.GetUTCNow().Equal(stopped), "a closed clock must not refresh, even after SetInterval")
}

// TestClockSetIntervalTakesEffectImmediately verifies that shortening the refresh interval of a clock that is
// waiting out a one-hour interval makes the cached time advance right away instead of after that hour.
func TestClockSetIntervalTakesEffectImmediately(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewClock(ctx, time.Hour)
	created := c.GetUTCNow()
	// Let the refresh loop start waiting out the hour. The assertion below does not depend on this pause; it
	// only makes sure the loop already read the old interval, which is the situation under test.
	time.Sleep(50 * time.Millisecond)

	c.SetInterval(time.Millisecond)
	require.Equal(t, time.Millisecond, c.Interval())
	require.Eventually(t, func() bool { return c.GetUTCNow().After(created) },
		clockTestDeadline, time.Millisecond, "the new interval must apply without waiting for the old one")
}
