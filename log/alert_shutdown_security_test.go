package log

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// alertShutdownContext pauses one Err call after observing the current state.
// It models cancellation between the last lifecycle check and queue admission.
type alertShutdownContext struct {
	context.Context
	entered, release chan struct{}
	once             sync.Once
}

// Err returns the observed error after the test-controlled cancellation boundary.
func (c *alertShutdownContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.entered); <-c.release })
	return err
}

// alertShutdownLimiter keeps these lifecycle-only tests off every HTTP endpoint.
type alertShutdownLimiter struct{}

// Allow rejects payloads without blocking or invoking external code.
func (alertShutdownLimiter) Allow() bool { return false }

// TestSecurity38AdmissionShutdown orders shutdown against a paused admission.
// A successful send ordered before Close may be abandoned, but a send whose
// admission occurs after Close and Done must never report successful enqueueing.
func TestSecurity38AdmissionShutdown(t *testing.T) {
	for _, mode := range []string{"close", "parent"} {
		t.Run(mode, func(t *testing.T) {
			violations := 0
			for range 32 {
				parent, cancel := context.WithCancel(context.Background())
				a, err := NewAlert(parent, "https://example.invalid", WithAlertType("test"), WithAlertToken("synthetic"), WithRateLimiter(alertShutdownLimiter{}))
				require.NoError(t, err)
				paused := &alertShutdownContext{Context: a.ctx, entered: make(chan struct{}), release: make(chan struct{})}
				// runSender uses its constructor argument, not this admission-only wrapper.
				a.ctx = paused
				result := make(chan error, 1)
				go func() { result <- a.Send("synthetic queued alert") }()
				select {
				case <-paused.entered:
				case <-time.After(time.Second):
					close(paused.release)
					cancel()
					a.Close()
					t.Fatal("admission did not reach test boundary")
				}
				stopped := a.Done()
				if mode == "close" {
					returned := make(chan struct{})
					go func() { a.Close(); close(returned) }()
					stopped = returned
				} else {
					cancel()
				}
				before := false
				select {
				case <-stopped:
					before = true
				case <-time.After(5 * time.Millisecond):
					// Serialized admission intentionally keeps shutdown waiting for Err.
				}
				close(paused.release)
				select {
				case err := <-result:
					if before && err == nil {
						violations++
					}
				case <-time.After(time.Second):
					t.Fatal("admission did not return")
				}
				a.Close()
				cancel()
				select {
				case <-a.Done():
				case <-time.After(time.Second):
					t.Fatal("sender was not joined")
				}
			}
			require.Zero(t, violations, "orphaned queue accepted an alert after shutdown")
		})
	}
}

// alertCloseLimiter closes its owner from the worker callback without a lock cycle.
type alertCloseLimiter struct {
	close  func()
	called atomic.Int32
}

// Allow initiates shutdown reentrantly and records calls after cancellation.
func (l *alertCloseLimiter) Allow() bool { l.called.Add(1); l.close(); return false }

// TestSecurity38ShutdownDrainsQueue releases retained unsent payloads and prevents
// further rate-limit callbacks after a callback initiates shutdown itself.
func TestSecurity38ShutdownDrainsQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opt, err := new(alertOption).applyOpts()
	require.NoError(t, err)
	a := &Alert{alertOption: opt, ctx: ctx, cancel: cancel, done: make(chan struct{}), stopChan: make(chan struct{}), senderChan: make(chan *alertMsg, 64)}
	limiter := &alertCloseLimiter{close: a.Close}
	a.ratelimiter = limiter
	for range 64 {
		a.senderChan <- &alertMsg{alertType: "test", pushToken: "synthetic", msg: "not delivered"}
	}
	go a.runSender(ctx)
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("reentrant shutdown did not return")
	}
	require.EqualValues(t, 1, limiter.called.Load())
	require.Empty(t, a.senderChan, "shutdown retained queued message/token references")
	require.Error(t, a.SendWithType("test", "synthetic", "after done"))
}
