package log

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// stalledSecuritySender holds one send until the test or its context releases it.
type stalledSecuritySender struct {
	entered chan struct{}
	release chan struct{}
}

// Send acknowledges entry before waiting; cleanup always releases the sender.
func (s *stalledSecuritySender) Send(ctx context.Context, _ []byte) error {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestSecurity63StalledSenderReturns reproduces a full queue under a live context.
func TestSecurity63StalledSenderReturns(t *testing.T) {
	for _, capacity := range []int{0, 1} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			sender := &stalledSecuritySender{make(chan struct{}, 1), make(chan struct{})}
			p, err := NewPusher(ctx, WithPusherSender(sender), WithPusherSenderChanLen(capacity))
			require.NoError(t, err)
			defer cancel()
			defer close(sender.release)
			p.senderChan <- []byte("first")
			<-sender.entered
			if capacity > 0 {
				p.senderChan <- []byte("queued")
			}
			done := make(chan struct{})
			go func() { defer close(done); _ = p.GetZapHook()(zapcore.Entry{Message: "bounded"}, nil) }()
			select {
			case <-done:
			case <-time.After(200 * time.Millisecond):
				t.Error("hook waits for a stalled sender while lifecycle context remains active")
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("hook did not stop on cleanup")
				}
			}
		})
	}
}

// securityRoundTripper supplies deterministic non-network HTTP behavior.
type securityRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the test-controlled transport.
func (f securityRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestSecurity63HTTPDeadline requires a finite deadline even with a zero-timeout client.
func TestSecurity63HTTPDeadline(t *testing.T) {
	c := &http.Client{Transport: securityRoundTripper(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		require.True(t, ok, "HTTP sender must supply a finite deadline")
		if ok {
			require.LessOrEqual(t, time.Until(deadline), 30*time.Second)
		}
		return nil, errors.New("synthetic failure")
	})}
	require.Error(t, NewPusherHTTPSender(c, "https://example.invalid", nil).Send(context.Background(), nil))
}

// TestSecurity49PusherRedactsErrors checks nested and malformed URL diagnostics.
func TestSecurity49PusherRedactsErrors(t *testing.T) {
	marker := "SYNTHETIC_SECRET_49"
	for _, endpoint := range []string{
		"https://user:" + marker + "@example.invalid/path/" + marker + "?token=" + marker + "#" + marker,
		"https://[::1]/" + marker + "?signature=" + marker,
		"https://" + marker + "\n.invalid",
	} {
		t.Run(fmt.Sprint(len(endpoint)), func(t *testing.T) {
			c := &http.Client{Transport: securityRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "synthetic", URL: endpoint, Err: fmt.Errorf("echo: %s", marker)}
			})}
			err := NewPusherHTTPSender(c, endpoint, nil).Send(context.Background(), nil)
			require.Error(t, err)
			for err != nil {
				for _, format := range []string{"%v", "%+v", "%#v"} {
					require.NotContains(t, fmt.Sprintf(format, err), marker)
				}
				err = errors.Unwrap(err)
			}
		})
	}
}

// TestSecurity63LifecycleAndBudgets exercises cancellation, failures and validation.
func TestSecurity63LifecycleAndBudgets(t *testing.T) {
	_, err := NewPusher(nil)
	require.Error(t, err)
	for _, opt := range []PusherOption{WithPusherSendTimeout(0), WithPusherMaxMessageBytes(0), WithPusherSenderChanLen(-1)} {
		_, err := NewPusher(context.Background(), opt)
		require.Error(t, err)
	}
	sender := &stalledSecuritySender{make(chan struct{}, 1), make(chan struct{})}
	p, err := NewPusher(context.Background(), WithPusherSender(sender), WithPusherSendTimeout(20*time.Millisecond), WithPusherMaxMessageBytes(512))
	require.NoError(t, err)
	defer close(sender.release)
	defer p.Close()
	require.NoError(t, p.GetZapHook()(zapcore.Entry{Message: "tiny"}, nil))
	select {
	case <-sender.entered:
	case <-time.After(time.Second):
		t.Fatal("sender never received queued entry")
	}
	require.Eventually(t, func() bool { return p.Stats().Failed == 1 }, time.Second, time.Millisecond)
	require.Equal(t, uint64(1), p.Stats().Enqueued)
	require.ErrorIs(t, p.GetZapHook()(zapcore.Entry{Message: string(make([]byte, 1024))}, nil), ErrPusherMessageTooLarge)
	require.Equal(t, uint64(1), p.Stats().Dropped)
	p.Close()
	p.Close()
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("sender did not terminate")
	}
	require.ErrorIs(t, p.GetZapHook()(zapcore.Entry{}, nil), ErrPusherClosed)
	require.Equal(t, uint64(2), p.Stats().Dropped)
}

// TestSecurity63EarlierHTTPDeadline retains caller cancellation and error classification.
func TestSecurity63EarlierHTTPDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	original, _ := parent.Deadline()
	c := &http.Client{Transport: securityRoundTripper(func(r *http.Request) (*http.Response, error) {
		got, ok := r.Context().Deadline()
		require.True(t, ok)
		require.Equal(t, original, got)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	s := NewPusherHTTPSender(c, "https://example.invalid/secret", nil)
	require.ErrorIs(t, s.Send(parent, nil), context.DeadlineExceeded)
	require.Error(t, s.Send(nil, nil))
	require.NotPanics(t, func() { require.Error(t, NewPusherHTTPSender(nil, ":invalid", nil).Send(context.Background(), nil)) })
}

// sharedSecurityFormatter deliberately returns a caller-owned buffer to test queue ownership.
type sharedSecurityFormatter struct{ body []byte }

// Format returns the reusable fixture buffer unchanged.
func (f *sharedSecurityFormatter) Format(zapcore.Entry, []zapcore.Field) ([]byte, error) {
	return f.body, nil
}

// TestSecurity63QueueOwnsPayload verifies that producer buffer reuse cannot alter queued bytes.
func TestSecurity63QueueOwnsPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &stalledSecuritySender{make(chan struct{}, 1), make(chan struct{})}
	formatter := &sharedSecurityFormatter{[]byte("ORIGINAL")}
	p, err := NewPusher(ctx, WithPusherSender(sender), WithPusherFormatter(formatter), WithPusherSenderChanLen(1))
	require.NoError(t, err)
	defer p.Close()
	defer close(sender.release)
	p.senderChan <- []byte("held")
	<-sender.entered
	require.NoError(t, p.GetZapHook()(zapcore.Entry{}, nil))
	copy(formatter.body, []byte("MODIFIED"))
	require.Equal(t, "ORIGINAL", string(<-p.senderChan))
}
