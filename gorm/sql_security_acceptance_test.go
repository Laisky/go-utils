package gorm

import (
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// securitySQLMethodTrap detects unintended invocation of arbitrary formatter methods.
type securitySQLMethodTrap struct{}

// String panics if a diagnostic attempts to render untrusted formatter objects.
func (securitySQLMethodTrap) String() string { panic("formatter String method must not run") }

// TestSecurity60UnsafeModeIsExplicit verifies disclosure requires a constructor option and never enables suppression.
func TestSecurity60UnsafeModeIsExplicit(t *testing.T) {
	const query = "INSERT INTO samples VALUES ('SYNTHETIC_SECRET/*disable_log*/')"
	for _, unsafe := range []bool{false, true} {
		capture := &securitySQLCapture{t: t}
		var options []LoggerOption
		if unsafe {
			options = append(options, WithUnsafeSQLLogging())
		}
		logger := NewLogger(func(...any) []any { return []any{"", "", "", query} }, capture, options...)
		logger.Print("sql", "SYNTHETIC_CALLER", time.Millisecond, query, []any{"SYNTHETIC_ARGUMENT"}, 1, "SYNTHETIC_EXTRA")
		require.Len(t, capture.entries, 1)
		if unsafe {
			require.Contains(t, capture.entries[0], "SYNTHETIC_SECRET")
		} else {
			require.NotContains(t, capture.entries[0], "SYNTHETIC_SECRET")
		}
		for _, secret := range []string{"SYNTHETIC_CALLER", "SYNTHETIC_ARGUMENT", "SYNTHETIC_EXTRA"} {
			require.NotContains(t, capture.entries[0], secret)
		}
	}
}

// TestSecurity60ArbitraryValuesCannotRender keeps Stringer values and unexpected row counts out of logs.
func TestSecurity60ArbitraryValuesCannotRender(t *testing.T) {
	for _, value := range []any{securitySQLMethodTrap{}, map[string]any{"secret": "SENTINEL"}, []any{"SENTINEL"}} {
		capture := &securitySQLCapture{t: t}
		logger := NewLogger(func(...any) []any { return []any{"", "", "", value} }, capture)
		require.NotPanics(t, func() { logger.Print(value, value, time.Millisecond, value, value, value, value) })
		require.Len(t, capture.entries, 1)
		require.NotContains(t, capture.entries[0], "SENTINEL")
		require.NotContains(t, capture.entries[0], "affected")
	}
}

// TestSecurity60MetadataBounds covers nil/short formatters, multiline classification, row types and verbose byte limits.
func TestSecurity60MetadataBounds(t *testing.T) {
	for _, rows := range []any{int(2), int8(2), int16(2), int32(2), int64(2), uint(2), uint8(2), uint16(2), uint32(2), uint64(2)} {
		capture := &securitySQLCapture{t: t}
		NewLogger(nil, capture).Print("sql", "caller", time.Millisecond, " \nSeLeCt\tvalue FROM samples", nil, rows)
		require.Equal(t, zapcore.DebugLevel, capture.levels[0])
		require.Contains(t, capture.entries[0], `"affected":2`)
	}
	capture := &securitySQLCapture{t: t}
	NewLogger(nil, capture, nil).Print()
	require.Len(t, capture.entries, 1)
	require.Contains(t, capture.entries[0], `"operation":"unknown"`)
	query := strings.Repeat("x", 20*1024) + "SENTINEL_AFTER_LIMIT"
	capture = &securitySQLCapture{t: t}
	NewLogger(nil, capture, WithUnsafeSQLLogging()).Print("sql", "caller", time.Second, query)
	require.Len(t, capture.entries, 1)
	require.Less(t, len(capture.entries[0]), 17*1024)
	require.NotContains(t, capture.entries[0], "SENTINEL_AFTER_LIMIT")
}
