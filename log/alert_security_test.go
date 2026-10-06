package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Laisky/zap/buffer"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// alertSecurityObject counts calls so rejected context is never serialized.
type alertSecurityObject struct{ calls *atomic.Int64 }

// MarshalLogObject must not run under the default remote field policy.
func (o alertSecurityObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	o.calls.Add(1)
	enc.AddString("Authorization", "SYNTHETIC_NESTED_38")
	return nil
}

// alertSecurityArray verifies array marshalers are excluded before execution.
type alertSecurityArray struct{ calls *atomic.Int64 }

// MarshalLogArray must not run for a default alert.
func (o alertSecurityArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	o.calls.Add(1)
	enc.AppendString("SYNTHETIC_ARRAY_38")
	return nil
}

// captureSecurityAlert starts a local, bounded GraphQL fixture and joins it in cleanup.
func captureSecurityAlert(t *testing.T, options ...AlertOption) (*Alert, <-chan string) {
	t.Helper()
	messages := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			Variables struct {
				Msg string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		select {
		case messages <- request.Variables.Msg:
		default:
			t.Error("unexpected extra alert")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, err := NewAlert(ctx, server.URL, append([]AlertOption{WithAlertType("synthetic"), WithAlertToken("synthetic-auth")}, options...)...)
	require.NoError(t, err)
	t.Cleanup(func() {
		a.Close()
		select {
		case <-a.Done():
		case <-time.After(2 * time.Second):
			t.Error("alert worker did not exit")
		}
	})
	return a, messages
}

// TestSecurity38DefaultFields proves that remote alerts do not inherit private log context.
func TestSecurity38DefaultFields(t *testing.T) {
	a, messages := captureSecurityAlert(t)
	var calls atomic.Int64
	entry := zapcore.Entry{Level: zap.ErrorLevel, Message: "safe operation failed", Stack: "SYNTHETIC_STACK_38"}
	fields := []zapcore.Field{
		zap.String("api_token", "SYNTHETIC_FLAT_38"),
		zap.Any("context", map[string]any{"Authorization": "SYNTHETIC_REFLECT_38"}),
		zap.Error(fmt.Errorf("SYNTHETIC_ERROR_38")),
		zap.Object("nested", alertSecurityObject{&calls}),
		zap.Array("array", alertSecurityArray{&calls}),
	}
	require.NoError(t, a.GetZapHook()(entry, fields))
	select {
	case message := <-messages:
		require.Contains(t, message, "safe operation failed")
		require.NotContains(t, message, "SYNTHETIC_", "default payload must exclude fields and stack")
	case <-time.After(3 * time.Second):
		t.Fatal("no local alert")
	}
	require.Zero(t, calls.Load(), "unapproved marshalers must not execute")
}

// TestSecurity38Allowlist keeps approved scalars while excluding every executable/nested field.
func TestSecurity38Allowlist(t *testing.T) {
	keys := []string{"request_id", "status", "context", "error", "nested", "array"}
	a, messages := captureSecurityAlert(t, WithAlertFieldAllowlist(keys...))
	keys[0] = "api_token" // caller mutation must not change the installed policy.
	var calls atomic.Int64
	fields := []zapcore.Field{
		zap.String("request_id", "request-38"), zap.Int("status", 503),
		zap.String("api_token", "SYNTHETIC_TOKEN"),
		zap.Error(fmt.Errorf("SYNTHETIC_ERROR")),
		zap.Object("nested", alertSecurityObject{&calls}),
		zap.Array("array", alertSecurityArray{&calls}),
		zap.Any("context", map[string]any{"Authorization": "SYNTHETIC_REFLECT"}),
		zap.Namespace("private"), zap.String("request_id", "SYNTHETIC_NAMESPACED"),
	}
	require.NoError(t, a.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "approved"}, fields))
	select {
	case message := <-messages:
		require.Contains(t, message, `"request_id":"request-38"`)
		require.Contains(t, message, `"status":503`)
		require.NotContains(t, message, "SYNTHETIC")
	case <-time.After(3 * time.Second):
		t.Fatal("no approved alert")
	}
	require.Zero(t, calls.Load())
}

// failingAlertEncoder injects an encoder failure without a raw-field fallback.
type failingAlertEncoder struct{ zapcore.Encoder }

// EncodeEntry returns synthetic secret-bearing error text that must be discarded.
func (f failingAlertEncoder) EncodeEntry(zapcore.Entry, []zapcore.Field) (*buffer.Buffer, error) {
	return nil, fmt.Errorf("SYNTHETIC_ENCODER_SECRET")
}

// TestSecurity38BudgetsAndEncoding checks early rejection and no fallback delivery.
func TestSecurity38BudgetsAndEncoding(t *testing.T) {
	for _, opt := range []AlertOption{WithAlertMaxMessageBytes(0), WithAlertMaxMessageBytes(1<<20 + 1), WithAlertPushTimeout(0), WithAlertFieldAllowlist(""), WithAlertFieldAllowlist(strings.Repeat("x", 129))} {
		_, err := NewAlert(context.Background(), "https://example.invalid", opt)
		require.Error(t, err)
	}
	a, messages := captureSecurityAlert(t, WithAlertMaxMessageBytes(256), WithAlertFieldAllowlist("request_id"))
	require.ErrorIs(t, a.Send(strings.Repeat("x", 257)), ErrAlertMessageTooLarge)
	hook := a.GetZapHook()
	require.ErrorIs(t, hook(zapcore.Entry{Level: zap.ErrorLevel, Message: strings.Repeat("x", 257)}, nil), ErrAlertMessageTooLarge)
	require.ErrorIs(t, hook(zapcore.Entry{Level: zap.ErrorLevel}, []zapcore.Field{zap.String("request_id", strings.Repeat("x", 257))}), ErrAlertMessageTooLarge)
	a.encPool = &sync.Pool{New: func() any { return failingAlertEncoder{zapcore.NewJSONEncoder(zapcore.EncoderConfig{})} }}
	err := hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "not sent"}, nil)
	require.Error(t, err)
	require.NotContains(t, fmt.Sprintf("%+v", err), "SYNTHETIC")
	require.Empty(t, messages)
	require.Empty(t, a.senderChan)
}

// alertDenyLimiter acknowledges a dropped payload without inspecting its contents.
type alertDenyLimiter struct{ called chan struct{} }

// Allow denies the alert and signals the deterministic test boundary.
func (d alertDenyLimiter) Allow() bool { close(d.called); return false }

// alertLogBuffer makes asynchronous diagnostic capture race-safe.
type alertLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

// Write serializes writes from the alert worker.
func (b *alertLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

// String returns a synchronized copy of captured diagnostics.
func (b *alertLogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Buffer.String() }

// TestSecurity38Diagnostics never re-logs payloads, labels, tokens, or remote errors.
func TestSecurity38Diagnostics(t *testing.T) {
	const secret = "SYNTHETIC_DIAGNOSTIC_38"
	var captured alertLogBuffer
	original := Shared
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&captured), zap.DebugLevel)
	})))
	require.NoError(t, err)
	Shared = logger
	defer func() { Shared = original }()
	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		http.Error(w, secret, http.StatusBadRequest)
	}))
	defer server.Close()
	for _, denied := range []bool{false, true} {
		deniedCall := make(chan struct{})
		options := []AlertOption{WithAlertToken(secret), WithAlertType(secret)}
		if denied {
			options = append(options, WithRateLimiter(alertDenyLimiter{deniedCall}))
		}
		a, err := NewAlert(context.Background(), server.URL+"/"+secret+"?token="+secret, options...)
		require.NoError(t, err)
		require.NoError(t, a.Send(secret))
		if denied {
			select {
			case <-deniedCall:
			case <-time.After(time.Second):
				t.Fatal("limiter not called")
			}
		} else {
			select {
			case <-called:
			case <-time.After(time.Second):
				t.Fatal("request not delivered")
			}
		}
		a.Close()
		select {
		case <-a.Done():
		case <-time.After(time.Second):
			t.Fatal("sender did not exit")
		}
	}
	require.NotContains(t, captured.String(), secret)
}

// TestSecurity38CloseCancelsDelivery waits for actual HTTP entry then closes the worker.
func TestSecurity38CloseCancelsDelivery(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	a, err := NewAlert(context.Background(), server.URL, WithAlertType("test"), WithAlertToken("test"))
	require.NoError(t, err)
	defer a.Close()
	require.NoError(t, a.Send("test"))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no request")
	}
	a.Close()
	a.Close()
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel in-flight request")
	}
	require.Error(t, a.Send("after close"))
}
