package log

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

// TestAlertHook verifies field-aware alert delivery against a local GraphQL fixture.
func TestAlertHook(t *testing.T) {
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode alert request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"TelegramMonitorAlert": map[string]any{"name": "accepted"}},
		}); err != nil {
			t.Errorf("encode alert response: %v", err)
		}
	}))
	defer server.Close()
	pusher, err := NewAlert(context.Background(), server.URL,
		WithAlertType("compatibility"), WithAlertToken("test-token"))
	require.NoError(t, err)
	defer pusher.Close()
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(
		zap.Fields(zap.String("bound", "context")),
		zap.HooksWithFields(pusher.GetZapHook()),
	))
	require.NoError(t, err)
	logger.Info("below alert level")
	logger.Error("compatibility alert", zap.String("call", "value"), zap.Error(errors.Errorf("test error")))
	select {
	case request := <-requests:
		variables, ok := request["variables"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "compatibility", variables["type"])
		require.Equal(t, "test-token", variables["token"])
		message, ok := variables["msg"].(string)
		require.True(t, ok)
		require.Contains(t, message, "compatibility alert")
		require.Contains(t, message, `"bound":"context"`)
		require.Contains(t, message, `"call":"value"`)
		require.Contains(t, message, `"error":"test error"`)
	case <-time.After(5 * time.Second):
		t.Fatal("local alert was not delivered")
	}
	require.Empty(t, requests, "below-level entries must not enqueue an alert")
}

// TestAlert_SendAfterClose verifies that sending after Close returns an error
// instead of panicking with "send on closed channel", and that Close is
// idempotent (a second Close must not panic with "close of closed channel").
func TestAlert_SendAfterClose(t *testing.T) {
	a, err := NewAlert(
		context.Background(),
		"https://gq.laisky.com/query/",
		WithAlertType("hello"),
		WithAlertToken("YOUR_ALERT_TOKEN"),
	)
	require.NoError(t, err)

	a.Close()

	// Send after Close must return an error and must not panic.
	err = a.Send("x")
	require.Error(t, err)

	// SendWithType after Close must also return an error and not panic.
	err = a.SendWithType("hello", "YOUR_ALERT_TOKEN", "x")
	require.Error(t, err)

	// A second Close must be a no-op, not a panic.
	require.NotPanics(t, func() { a.Close() })
}

func ExampleAlert() {
	pusher, err := NewAlert(
		context.Background(),
		"https://gq.laisky.com/query/",
		WithAlertType("hello"),
		WithAlertToken("YOUR_ALERT_TOKEN"),
		WithAlertHookLevel(zap.InfoLevel),
	)
	if err != nil {
		Shared.Panic("create alert pusher", zap.Error(err))
	}
	defer pusher.Close()
	logger := Shared.WithOptions(
		zap.Fields(zap.String("logger", "test")),
		zap.HooksWithFields(pusher.GetZapHook()),
	)

	logger.Debug("DEBUG", zap.String("yo", "hello"))
	logger.Info("Info", zap.String("yo", "hello"))
	logger.Warn("Warn", zap.String("yo", "hello"))
	logger.Error("Error", zap.String("yo", "hello"))

	time.Sleep(1 * time.Second)
}
