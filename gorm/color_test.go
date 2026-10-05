package gorm

import (
	"testing"
	"time"

	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// TestGormLogger_Print preserves operation classification for legacy formatter types and short output.
func TestGormLogger_Print(t *testing.T) {
	t.Run("gorm v1", func(t *testing.T) {
		logger := &securitySQLCapture{t: t}
		for _, msg := range []any{"drop", "delete", "insert", "update", "select", "error", []byte("drop"), 123} {
			formatter := func(...any) []any { return []any{"", "", "", msg} }
			NewLogger(formatter, logger).Print("type", "caller", time.Second, "sql", "args", "affected", "extras")
		}
		require.Len(t, logger.entries, 8)
		require.Equal(t, []zapcore.Level{zapcore.InfoLevel, zapcore.InfoLevel, zapcore.InfoLevel, zapcore.InfoLevel, zapcore.DebugLevel, zapcore.ErrorLevel, zapcore.InfoLevel, zapcore.InfoLevel}, logger.levels)
		for _, event := range logger.entries {
			require.NotContains(t, event, "extras")
			require.Contains(t, event, `"ms":1000`)
		}
	})
	t.Run("short", func(t *testing.T) {
		logger := &securitySQLCapture{t: t}
		NewLogger(func(...any) []any { return []any{"yo"} }, logger).Print("type", "caller", time.Second, "SELECT ?", "args", 3, "extras")
		require.Len(t, logger.entries, 1)
		require.Equal(t, zapcore.DebugLevel, logger.levels[0])
		require.Contains(t, logger.entries[0], `"affected":3`)
	})
}
