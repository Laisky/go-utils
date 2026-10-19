package log

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
)

const defaultAlertMessageBytes = 16 * 1024

// ErrAlertMessageTooLarge rejects an alert before it enters the remote queue.
var ErrAlertMessageTooLarge = errors.New("alert message exceeds byte limit")

// WithAlertFieldAllowlist selects the legacy explicit-filter policy, replacing
// faithful default forwarding. An empty list sends metadata only. It exports
// only named top-level string, bool,
// integer, float, and duration fields. Objects, arrays, errors, reflection,
// stringers, binary/byte strings, complex numbers, times, uintptrs, namespaces,
// and stack/caller data are never exported by this hook, and fields of unknown
// types are dropped. Use an approved scalar summary instead.
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

// remoteFields selects values before encoding; default errors become sanitized
// strings, while complex marshalers never execute. Explicit filters retain their policy.
func (a *Alert) remoteFields(fields []zapcore.Field) ([]zapcore.Field, error) {
	if a.allowedFields != nil && len(a.allowedFields) == 0 {
		return nil, nil
	}
	selected := make([]zapcore.Field, 0, min(len(fields), a.maxMessageBytes/32))
	remaining := a.maxMessageBytes
	for _, field := range fields {
		disposition := alertFieldDispositionOf(field.Type)
		if disposition == alertFieldStop {
			break
		}
		if a.allowedFields == nil {
			var ok bool
			field, ok = a.defaultDiagnosticField(field)
			if !ok {
				continue
			}
		} else {
			if disposition != alertFieldExport {
				continue
			}
			if _, ok := a.allowedFields[field.Key]; !ok {
				continue
			}
			if a.sanitizeDiagnosticText(field.Key) != field.Key {
				continue
			}
			if sensitiveDiagnosticKey(field.Key) {
				field.Type, field.String, field.Integer = zapcore.StringType, diagnosticRedacted, 0
			} else if field.Type == zapcore.StringType {
				field.String = a.sanitizeDiagnosticText(field.String)
			}
		}
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
	return selected, nil
}

// alertFieldDisposition is the allowlist policy's decision for one field type.
type alertFieldDisposition int

const (
	// alertFieldDrop excludes the field even when its key is allowlisted.
	alertFieldDrop alertFieldDisposition = iota
	// alertFieldExport exports the field when its key is allowlisted.
	alertFieldExport
	// alertFieldStop excludes the field and every field after it.
	alertFieldStop
)

// alertFieldDispositionOf decides how the remote alert policy treats a field of
// the given zap type, independently of its key. It takes the field type and
// returns alertFieldExport only for primitive scalars stored inline in the
// field, alertFieldStop for a namespace, and alertFieldDrop for everything else,
// including field types added by future zap versions (fail closed).
func alertFieldDispositionOf(fieldType zapcore.FieldType) alertFieldDisposition {
	switch fieldType {
	case zapcore.StringType, zapcore.BoolType, zapcore.Int64Type, zapcore.Int32Type,
		zapcore.Int16Type, zapcore.Int8Type, zapcore.Uint64Type, zapcore.Uint32Type,
		zapcore.Uint16Type, zapcore.Uint8Type, zapcore.Float64Type, zapcore.Float32Type,
		zapcore.DurationType:
		return alertFieldExport
	case zapcore.NamespaceType:
		// Namespace changes the scope of every later field; do not flatten nested
		// values into seemingly approved top-level names.
		return alertFieldStop
	case zapcore.ArrayMarshalerType, zapcore.ObjectMarshalerType, zapcore.InlineMarshalerType,
		zapcore.ReflectType, zapcore.StringerType, zapcore.ErrorType:
		// These carry caller code or arbitrary values whose rendered output is
		// unbounded and unreviewed. They are dropped before any method runs.
		return alertFieldDrop
	case zapcore.BinaryType, zapcore.ByteStringType, zapcore.Complex128Type, zapcore.Complex64Type,
		zapcore.TimeType, zapcore.TimeFullType, zapcore.UintptrType:
		// Raw bytes, complex numbers, timestamps with caller-supplied locations,
		// and memory addresses are outside the approved scalar summary policy.
		return alertFieldDrop
	case zapcore.UnknownType, zapcore.SkipType:
		// An uninitialized field panics when encoded, and a skip field is a no-op.
		return alertFieldDrop
	default:
		// Field types added by future zap versions fail closed.
		return alertFieldDrop
	}
}

// WithAlertDiagnosticRedaction explicitly selects the former default policy:
// sanitized messages, logger names and primitive diagnostics, excluding payloads
// and complex fields. It does not change an explicit WithAlertFieldAllowlist.
func WithAlertDiagnosticRedaction() AlertOption {
	return func(o *alertOption) error {
		o.diagnosticRedaction = true
		return nil
	}
}

// GetZapHook forwards the supplied entry message, logger name and supported Zap
// fields unchanged by default, using standard Zap JSON field representations.
// Explicit redaction or allowlist options retain their documented policies.
// Encoding failures and size overflow reject the entire entry before admission.
//
// Deprecated: use GetZapCore with zap.WrapCore and zapcore.NewTee for logger.With
// context. A hook receives fields after its enclosing core has encoded them;
// it cannot recover binding order or eager snapshots of mutable bound values.
func (a *Alert) GetZapHook() func(zapcore.Entry, []zapcore.Field) error {
	return func(entry zapcore.Entry, fields []zapcore.Field) error {
		if !a.level.Enabled(entry.Level) {
			return nil
		}
		if err := a.checkAlertEntry(entry); err != nil {
			return err
		}
		selected, err := a.selectAlertFields(fields)
		if err != nil {
			return err
		}
		enc := a.newAlertEncoder()
		if err = addAlertFields(enc, selected, a.maxMessageBytes); err != nil {
			return err
		}
		return a.sendAlertEntry(entry, enc)
	}
}
