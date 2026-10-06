package log

import (
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
)

const defaultAlertMessageBytes = 16 * 1024

// ErrAlertMessageTooLarge rejects an alert before it enters the remote queue.
var ErrAlertMessageTooLarge = errors.New("alert message exceeds byte limit")

// WithAlertFieldAllowlist explicitly exports only named top-level scalar fields.
// Objects, arrays, errors, reflection, stringers, namespaces, and stack/caller
// data are never exported by this hook. Use an approved scalar summary instead.
// Free-form entry messages remain the caller's responsibility.
func WithAlertFieldAllowlist(keys ...string) AlertOption {
	return func(o *alertOption) error {
		if len(keys) > 256 {
			return errors.New("too many alert field names")
		}
		selected := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			if key == "" || len(key) > 128 {
				return errors.New("alert field names must contain 1 to 128 bytes")
			}
			selected[key] = struct{}{}
		}
		o.allowedFields = selected
		return nil
	}
}

// WithAlertMaxMessageBytes sets a 1-byte to 1-MiB cap for hook and direct messages.
// The default is 16 KiB. Oversized messages are rejected, never silently truncated.
func WithAlertMaxMessageBytes(limit int) AlertOption {
	return func(o *alertOption) error {
		if limit < 1 || limit > 1<<20 {
			return errors.New("alert message limit must be between 1 and 1048576 bytes")
		}
		o.maxMessageBytes = limit
		return nil
	}
}

// Done closes after the sender returns. Close cancels delivery rather than flushing.
func (a *Alert) Done() <-chan struct{} { return a.done }

// remoteFields selects values before encoding and never invokes user marshalers.
func (a *Alert) remoteFields(fields []zapcore.Field) ([]zapcore.Field, error) {
	if len(a.allowedFields) == 0 {
		return nil, nil
	}
	selected := make([]zapcore.Field, 0, min(len(fields), len(a.allowedFields)))
	remaining := a.maxMessageBytes
	for _, field := range fields {
		// Namespace changes the scope of every later field; do not flatten nested
		// values into seemingly approved top-level names.
		if field.Type == zapcore.NamespaceType {
			break
		}
		if _, ok := a.allowedFields[field.Key]; !ok {
			continue
		}
		switch field.Type {
		case zapcore.StringType, zapcore.BoolType, zapcore.Int64Type, zapcore.Int32Type,
			zapcore.Int16Type, zapcore.Int8Type, zapcore.Uint64Type, zapcore.Uint32Type,
			zapcore.Uint16Type, zapcore.Uint8Type, zapcore.Float64Type, zapcore.Float32Type,
			zapcore.DurationType:
			for _, size := range []int{len(field.Key), len(field.String), 32} {
				if size > remaining {
					return nil, errors.WithStack(ErrAlertMessageTooLarge)
				}
				remaining -= size
			}
			// Keep only primitive storage. Even a manually constructed field cannot
			// carry an unexpected object through the selected Interface member.
			field.Interface = nil
			selected = append(selected, field)
		}
	}
	return selected, nil
}

// GetZapHook sends minimal metadata and explicitly allowed scalar fields only.
// Entry.Message and LoggerName are application-authored text, not automatically
// redacted. Do not put secrets in them or in an allowlisted scalar value.
func (a *Alert) GetZapHook() func(zapcore.Entry, []zapcore.Field) error {
	return func(entry zapcore.Entry, fields []zapcore.Field) error {
		if !a.level.Enabled(entry.Level) {
			return nil
		}
		if a.closed.Load() || a.ctx.Err() != nil {
			return errors.New("alert is closed")
		}
		if len(entry.Message) > a.maxMessageBytes || len(entry.LoggerName) > a.maxMessageBytes-len(entry.Message) {
			return errors.WithStack(ErrAlertMessageTooLarge)
		}
		selected, err := a.remoteFields(fields)
		if err != nil {
			return err
		}
		enc, ok := a.encPool.Get().(zapcore.Encoder)
		if !ok {
			return errors.New("invalid alert encoder")
		}
		defer a.encPool.Put(enc)
		// Metadata is composed explicitly, never inherited from encoder options.
		buffer, err := enc.EncodeEntry(zapcore.Entry{}, selected)
		if buffer != nil {
			defer buffer.Free()
		}
		if err != nil || buffer == nil {
			return errors.New("encode approved alert fields failed")
		}
		msg := "logger: `" + entry.LoggerName + "`\n" +
			"time: `" + entry.Time.UTC().Format(time.RFC3339Nano) + "`\n" +
			"level: `" + entry.Level.String() + "`\n" +
			"message: `" + entry.Message + "`\n" + strings.TrimSpace(buffer.String())
		return a.Send(msg)
	}
}
