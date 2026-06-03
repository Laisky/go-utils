package log

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

func TestAlertHook(t *testing.T) {
	pusher, err := NewAlert(
		context.Background(),
		"https://gq.laisky.com/query/",
		WithAlertType("hello"),
		WithAlertToken("YOUR_ALERT_TOKEN"),
	)
	require.NoError(t, err)

	defer pusher.Close()
	logger := Shared.WithOptions(
		zap.Fields(zap.String("logger", "test")),
		zap.HooksWithFields(pusher.GetZapHook()),
	)

	logger.Debug("DEBUG", zap.String("yo", "hello"))
	logger.Info("Info", zap.String("yo", "hello"))
	logger.Warn("Warn", zap.String("yo", "hello"))
	logger.Error("Error", zap.String("yo", "hello"), zap.Bool("bool", true), zap.Error(errors.Errorf("xxx")))
	// t.Error()

	// time.Sleep(1 * time.Second)
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
