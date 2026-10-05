package gorm

import (
	"strings"
	"time"

	"github.com/Laisky/zap"
)

// loggerItf is the structured logging interface used by the GORM adapter.
type loggerItf interface {
	Debug(string, ...zap.Field)
	Info(string, ...zap.Field)
	Error(string, ...zap.Field)
}

// Logger emits operation metadata without SQL text, parameters, or arbitrary
// formatter fields by default. Query text never controls whether an event logs.
// Configure the logger before sharing it between goroutines.
type Logger struct {
	logger    loggerItf
	formatter func(...any) []any
	unsafeSQL bool
}

// LoggerOption configures a Logger at construction time.
type LoggerOption func(*Logger)

// WithUnsafeSQLLogging explicitly includes up to 16 KiB of formatted SQL in
// messages. This can expose credentials and personal data even at Info level.
// Use only with trusted data, restricted sinks and appropriately short retention.
// It never enables SQL-text-controlled suppression or raw argument/extra fields.
func WithUnsafeSQLLogging() LoggerOption {
	return func(logger *Logger) { logger.unsafeSQL = true }
}

// NewLogger creates a GORM adapter using formatter and logger. With no options,
// only allowlisted operation, duration and numeric affected-row metadata logs.
// A nil formatter uses the original SQL argument only to classify the operation.
func NewLogger(formatter func(...any) []any, logger loggerItf, options ...LoggerOption) *Logger {
	result := &Logger{logger: logger, formatter: formatter}
	for _, option := range options {
		if option != nil {
			option(result)
		}
	}
	return result
}

// Print records one operation. Default messages and fields cannot include bound
// values, raw errors, caller paths, or arbitrary extras. SELECT stays at Debug,
// errors at Error, and other operations at Info. No SQL marker disables logging.
func (l *Logger) Print(vs ...any) {
	var formatted any
	if len(vs) > 3 {
		formatted = vs[3]
	}
	if l.formatter != nil {
		values := l.formatter(vs...)
		if len(values) > 3 {
			formatted = values[3]
		}
	}
	operation := sqlOperation(formatted)
	if len(vs) > 0 && sqlOperation(vs[0]) == "error" {
		operation = "error"
	}
	fields := []zap.Field{zap.String("operation", operation)}
	if len(vs) > 2 {
		if duration, ok := vs[2].(time.Duration); ok {
			fields = append(fields, zap.Int64("ms", int64(duration/time.Millisecond)))
		}
	}
	if len(vs) > 5 {
		if rows, ok := sqlAffectedRows(vs[5]); ok {
			fields = append(fields, rows)
		}
	}
	message := "database operation"
	if l.unsafeSQL {
		if text := sqlTextPrefix(formatted, 16*1024); text != "" {
			message = text
		}
	}
	switch operation {
	case "select":
		l.logger.Debug(message, fields...)
	case "error":
		l.logger.Error(message, fields...)
	default:
		l.logger.Info(message, fields...)
	}
}

// sqlTextPrefix reads only bounded string/byte content, never invoking methods on
// arbitrary formatter values. It returns at most limit bytes for classification
// or the explicitly enabled unsafe diagnostics.
func sqlTextPrefix(value any, limit int) string {
	switch value := value.(type) {
	case string:
		return value[:min(len(value), limit)]
	case []byte:
		return string(value[:min(len(value), limit)])
	default:
		return ""
	}
}

// sqlOperation maps a bounded first token to an allowlisted label. Unknown text
// becomes "unknown" instead of entering a log as a user-controlled label.
func sqlOperation(value any) string {
	if _, ok := value.(error); ok {
		return "error"
	}
	text := strings.TrimLeft(sqlTextPrefix(value, 256), " \t\r\n")
	end := strings.IndexAny(text, " \t\r\n(")
	if end >= 0 {
		text = text[:end]
	}
	if len(text) > 16 {
		return "unknown"
	}
	switch token := strings.ToLower(text); token {
	case "select", "insert", "update", "delete", "drop", "create", "alter", "begin", "commit", "rollback", "error":
		return token
	default:
		return "unknown"
	}
}

// sqlAffectedRows accepts only builtin numeric row counts, excluding Stringer,
// marshaler and arbitrary values that could carry sensitive formatted content.
func sqlAffectedRows(value any) (zap.Field, bool) {
	switch value := value.(type) {
	case int:
		return zap.Int("affected", value), true
	case int8:
		return zap.Int64("affected", int64(value)), true
	case int16:
		return zap.Int64("affected", int64(value)), true
	case int32:
		return zap.Int64("affected", int64(value)), true
	case int64:
		return zap.Int64("affected", value), true
	case uint:
		return zap.Uint("affected", value), true
	case uint8:
		return zap.Uint64("affected", uint64(value)), true
	case uint16:
		return zap.Uint64("affected", uint64(value)), true
	case uint32:
		return zap.Uint64("affected", uint64(value)), true
	case uint64:
		return zap.Uint64("affected", value), true
	default:
		return zap.Field{}, false
	}
}
