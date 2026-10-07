package log

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// alertFieldTypeRecorder implements every executable zap value interface and
// counts invocations, so tests can prove that dropped field types never run
// caller code while being filtered.
type alertFieldTypeRecorder struct{ calls *atomic.Int64 }

// MarshalLogObject records an invocation and writes a synthetic secret.
func (r alertFieldTypeRecorder) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	r.calls.Add(1)
	enc.AddString("secret", "SYNTHETIC_FIELD_TYPE_OBJECT")
	return nil
}

// MarshalLogArray records an invocation and writes a synthetic secret.
func (r alertFieldTypeRecorder) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	r.calls.Add(1)
	enc.AppendString("SYNTHETIC_FIELD_TYPE_ARRAY")
	return nil
}

// String records an invocation and returns a synthetic secret.
func (r alertFieldTypeRecorder) String() string {
	r.calls.Add(1)
	return "SYNTHETIC_FIELD_TYPE_STRINGER"
}

// Error records an invocation and returns a synthetic secret.
func (r alertFieldTypeRecorder) Error() string {
	r.calls.Add(1)
	return "SYNTHETIC_FIELD_TYPE_ERROR"
}

// alertExportedFieldTypes lists the scalar field types that the allowlist
// policy exports. Every other zapcore.FieldType must be dropped.
var alertExportedFieldTypes = map[zapcore.FieldType]bool{
	zapcore.StringType: true, zapcore.BoolType: true, zapcore.DurationType: true,
	zapcore.Int64Type: true, zapcore.Int32Type: true, zapcore.Int16Type: true, zapcore.Int8Type: true,
	zapcore.Uint64Type: true, zapcore.Uint32Type: true, zapcore.Uint16Type: true, zapcore.Uint8Type: true,
	zapcore.Float64Type: true, zapcore.Float32Type: true,
}

// alertFieldTypesUnderTest returns every declared zapcore.FieldType plus two
// undeclared values that model field types added by a future zap release.
func alertFieldTypesUnderTest() []zapcore.FieldType {
	types := make([]zapcore.FieldType, 0, int(zapcore.InlineMarshalerType)+3)
	for fieldType := zapcore.UnknownType; fieldType <= zapcore.InlineMarshalerType; fieldType++ {
		types = append(types, fieldType)
	}
	return append(types, zapcore.InlineMarshalerType+1, zapcore.FieldType(255))
}

// TestAlertRemoteFieldsHandlesEveryFieldType verifies the allowlist policy's
// deliberate disposition of each zapcore.FieldType: the 13 approved scalar
// types are exported with their Interface member cleared, every other declared
// or unknown type is dropped without invoking caller code, and a namespace
// stops all later fields. It guards the exhaustive-switch lint cleanup.
func TestAlertRemoteFieldsHandlesEveryFieldType(t *testing.T) {
	t.Parallel()
	a := &Alert{alertOption: &alertOption{
		allowedFields:   map[string]struct{}{"approved": {}, "after": {}},
		maxMessageBytes: defaultAlertMessageBytes,
	}}
	for _, fieldType := range alertFieldTypesUnderTest() {
		var calls atomic.Int64
		field := zapcore.Field{
			Key: "approved", Type: fieldType, Integer: 42,
			String: "value", Interface: alertFieldTypeRecorder{&calls},
		}
		after := zap.String("after", "kept")
		selected, err := a.remoteFields([]zapcore.Field{field, after})
		require.NoError(t, err, "field type %d", fieldType)
		require.Zero(t, calls.Load(), "field type %d must not run caller code", fieldType)
		switch {
		case fieldType == zapcore.NamespaceType:
			require.Empty(t, selected, "a namespace must stop all later fields")
		case alertExportedFieldTypes[fieldType]:
			require.Len(t, selected, 2, "field type %d must be exported", fieldType)
			require.Equal(t, fieldType, selected[0].Type)
			require.Equal(t, int64(42), selected[0].Integer)
			require.Nil(t, selected[0].Interface, "exported scalars must not carry objects")
		default:
			require.Equal(t, []zapcore.Field{after}, selected, "field type %d must be dropped", fieldType)
		}
	}
}

// TestAlertHookEncodesEveryFieldTypeSafely sends one alert per field type
// through the public Zap hook and verifies that encoding never panics (even for
// UnknownType, which zap refuses to encode), that dropped types never reach the
// remote message, and that no marshaler, Stringer or error method executes.
func TestAlertHookEncodesEveryFieldTypeSafely(t *testing.T) {
	t.Parallel()
	a, messages := captureSecurityAlert(t, WithAlertFieldAllowlist("approved"))
	hook := a.GetZapHook()
	for _, fieldType := range alertFieldTypesUnderTest() {
		var calls atomic.Int64
		field := zapcore.Field{
			Key: "approved", Type: fieldType, Integer: 7,
			String: "SYNTHETIC_FIELD_TYPE_STRING", Interface: alertFieldTypeRecorder{&calls},
		}
		if alertExportedFieldTypes[fieldType] {
			field.String = "visible"
		}
		require.NotPanics(t, func() {
			require.NoError(t, hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "typed"}, []zapcore.Field{field}))
		}, "field type %d", fieldType)
		select {
		case message := <-messages:
			require.NotContains(t, message, "SYNTHETIC_FIELD_TYPE", "field type %d", fieldType)
			require.Equal(t, alertExportedFieldTypes[fieldType], strings.Contains(message, `"approved":`),
				"field type %d export disposition", fieldType)
		case <-time.After(3 * time.Second):
			t.Fatalf("no alert for field type %d", fieldType)
		}
		require.Zero(t, calls.Load(), "field type %d must not run caller code", fieldType)
	}
}
