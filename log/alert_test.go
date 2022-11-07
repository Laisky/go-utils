package log

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors"
	zap "github.com/Laisky/zap"
)

func TestAlertHook(t *testing.T) {
	pusher, err := NewAlert(
		context.Background(),
		"https://blog.laisky.com/graphql/query/",
		WithAlertType("hello"),
		WithAlertToken("rwkpVuAgaBZQBASKndHK"),
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}
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

	time.Sleep(5 * time.Second)
}
func ExampleAlert() {
	pusher, err := NewAlert(
		context.Background(),
		"https://blog.laisky.com/graphql/query/",
		WithAlertType("hello"),
		WithAlertToken("rwkpVuAgaBZQBASKndHK"),
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
