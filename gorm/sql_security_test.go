package gorm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// securitySQLCapture records the complete rendered Zap message and structured fields at each level.
type securitySQLCapture struct {
	t       *testing.T
	entries []string
	levels  []zapcore.Level
}

// record encodes the actual fields so nested values and errors cannot hide from sentinel checks.
func (c *securitySQLCapture) record(level zapcore.Level, message string, fields []zap.Field) {
	c.t.Helper()
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	output, err := encoder.EncodeEntry(zapcore.Entry{Level: level, Message: message}, fields)
	require.NoError(c.t, err)
	c.entries = append(c.entries, output.String())
	c.levels = append(c.levels, level)
	output.Free()
}

// Debug captures a debug event without filtering it out of the security assertions.
func (c *securitySQLCapture) Debug(message string, fields ...zap.Field) {
	c.record(zapcore.DebugLevel, message, fields)
}

// Info captures an info event.
func (c *securitySQLCapture) Info(message string, fields ...zap.Field) {
	c.record(zapcore.InfoLevel, message, fields)
}

// Error captures an error event.
func (c *securitySQLCapture) Error(message string, fields ...zap.Field) {
	c.record(zapcore.ErrorLevel, message, fields)
}

// TestSecurity60DefaultSQLLogsExcludeValues checks every formatter branch and the complete rendered event for secrets.
func TestSecurity60DefaultSQLLogsExcludeValues(t *testing.T) {
	const secret = "SYNTHETIC_SQL_SECRET"
	for _, operation := range []string{"INSERT", "UPDATE", "DELETE", "DROP", "SELECT", "error", "unknown"} {
		for _, format := range []string{"string", "bytes", "short", "error-value"} {
			t.Run(operation+"/"+format, func(t *testing.T) {
				sql := operation + " INTO samples(value) VALUES ('" + secret + "')"
				formatter := func(...any) []any {
					switch format {
					case "short":
						return []any{secret}
					case "bytes":
						return []any{secret, secret, secret, []byte(sql)}
					case "error-value":
						return []any{secret, secret, secret, fmt.Errorf("error %s", secret)}
					default:
						return []any{secret, secret, secret, sql}
					}
				}
				capture := &securitySQLCapture{t: t}
				logger := NewLogger(formatter, capture)
				logger.Print(secret, secret, 5*time.Millisecond, sql, []any{secret}, int64(2), map[string]any{"nested": secret})
				require.Len(t, capture.entries, 1)
				require.NotContains(t, capture.entries[0], secret)
				require.Contains(t, capture.entries[0], `"ms":5`)
				require.Contains(t, capture.entries[0], `"affected":2`)
			})
		}
	}
}

// TestSecurity76SQLTextCannotSuppressLogs keeps marker-like query data from controlling event visibility.
func TestSecurity76SQLTextCannotSuppressLogs(t *testing.T) {
	for _, query := range []string{
		"DELETE FROM samples WHERE label='ordinary/*disable_log*/'",
		"/*disable_log*/ SELECT value FROM samples",
		"error duplicate value '/*disable_log*/'",
		"SELECT 'nested /*disable_log*/ value'",
	} {
		for _, asBytes := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", query, asBytes), func(t *testing.T) {
				capture := &securitySQLCapture{t: t}
				formatter := func(...any) []any {
					var text any = query
					if asBytes {
						text = []byte(query)
					}
					return []any{"", "", "", text}
				}
				NewLogger(formatter, capture).Print("sql", "local-test", time.Millisecond, query, nil, 1)
				require.Len(t, capture.entries, 1)
				require.NotContains(t, strings.Join(capture.entries, ""), "/*disable_log*/")
			})
		}
	}
}
