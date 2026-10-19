package log

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// alertCoverageObject authors a synthetic native object through Zap's public interface.
type alertCoverageObject func(zapcore.ObjectEncoder) error

// MarshalLogObject calls the chosen synthetic object serializer.
func (value alertCoverageObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return value(enc)
}

// alertCoverageArray authors a synthetic native array through Zap's public interface.
type alertCoverageArray func(zapcore.ArrayEncoder) error

// MarshalLogArray calls the chosen synthetic array serializer.
func (value alertCoverageArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	return value(enc)
}

// alertCoveragePrimitiveArray exercises native values against the ordinary Zap JSON oracle.
type alertCoveragePrimitiveArray struct{}

// MarshalLogArray preserves each primitive representation, including nonfinite floats and text bytes.
func (alertCoveragePrimitiveArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	enc.AppendBool(true)
	enc.AppendComplex128(complex(1.25, -2.5))
	enc.AppendComplex64(complex(3.5, 4.25))
	enc.AppendDuration(17 * time.Millisecond)
	enc.AppendFloat64(math.Inf(1))
	enc.AppendFloat32(float32(math.NaN()))
	enc.AppendInt(-17)
	enc.AppendInt64(9007199254740993)
	enc.AppendInt32(-32)
	enc.AppendInt16(-16)
	enc.AppendInt8(-8)
	enc.AppendTime(time.Date(2026, time.October, 18, 22, 30, 0, 123, time.UTC))
	enc.AppendUint(17)
	enc.AppendUint64(18446744073709551615)
	enc.AppendUint32(32)
	enc.AppendUint16(16)
	enc.AppendUint8(8)
	enc.AppendUintptr(42)
	enc.AppendString("original\nUnicode 原文")
	enc.AppendByteString([]byte("original byte string 😀"))
	return nil
}

// alertCoverageRichError declares useful rich text or a deliberately failing formatter.
type alertCoverageRichError struct{ panicFormat bool }

// Error supplies the application-chosen ordinary error text.
func (alertCoverageRichError) Error() string { return "original rich error" }

// Format supplies original verbose text and makes formatter panic handling observable.
func (value alertCoverageRichError) Format(state fmt.State, _ rune) {
	if value.panicFormat {
		panic("SYNTHETIC_CALLBACK_FORMATTER_PANIC")
	}
	if _, err := state.Write([]byte("original verbose error 原文")); err != nil {
		panic("SYNTHETIC_CALLBACK_FORMAT_WRITER_ERROR")
	}
}

// alertCoveragePanickingError deliberately fails its ordinary Error callback.
type alertCoveragePanickingError struct{}

// Error makes a nonnil error callback panic rather than return a replacement marker.
func (alertCoveragePanickingError) Error() string { panic("SYNTHETIC_CALLBACK_ERROR_PANIC") }

// alertCoverageGroup supplies the error-group interface recognized by the pinned Zap encoder.
type alertCoverageGroup struct {
	members      []error
	panicMembers bool
}

// Error preserves the original group diagnostic without evaluating member callbacks.
func (alertCoverageGroup) Error() string { return "original multi-error group" }

// Errors returns synthetic causes, including supported nil members, or fails explicitly.
func (value alertCoverageGroup) Errors() []error {
	if value.panicMembers {
		panic("SYNTHETIC_CALLBACK_GROUP_PANIC")
	}
	return value.members
}

// TestAlertCoverageNativeRepresentations sends complete primitive, nested and rich-error content over HTTP.
func TestAlertCoverageNativeRepresentations(t *testing.T) {
	nestedObject := alertCoverageObject(func(enc zapcore.ObjectEncoder) error {
		enc.AddByteString("text", []byte("原文 😀\n"))
		enc.AddComplex128("complex128", complex(1.25, -2.5))
		enc.AddComplex64("complex64", complex(3.5, 4.25))
		enc.AddInt("native_int", -17)
		enc.AddTime("time", time.Date(2026, time.October, 18, 22, 30, 0, 123, time.UTC))
		enc.AddUint("native_uint", 17)
		enc.AddUintptr("uintptr", 42)
		return nil
	})
	nested := alertCoverageArray(func(enc zapcore.ArrayEncoder) error {
		if err := enc.AppendArray(alertCoveragePrimitiveArray{}); err != nil {
			return errors.WithStack(err)
		}
		if err := enc.AppendObject(nestedObject); err != nil {
			return errors.WithStack(err)
		}
		if err := enc.AppendArray(alertCoverageArray(func(zapcore.ArrayEncoder) error { return nil })); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(enc.AppendObject(alertCoverageObject(func(zapcore.ObjectEncoder) error { return nil })))
	})
	fields := []zapcore.Field{
		zap.ByteString("body", []byte("chosen original body 原文")),
		zap.Array("nested", nested),
		zap.NamedError("rich", alertCoverageRichError{}),
		zap.NamedError("group", alertCoverageGroup{members: []error{nil, errors.New("original first cause"), alertCoverageRichError{}}}),
	}
	expected := fidelityStandardFields(t, fields)
	alert, messages := captureSecurityAlert(t)
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "original structured operation"}, fields))
	received := defaultAlertMessage(t, messages)
	require.Contains(t, received, ",9007199254740993,", "the exact int64 must remain an unquoted JSON number")
	require.Contains(t, received, ",18446744073709551615,", "the exact uint64 must remain an unquoted JSON number")
	actual := fidelityAlertFields(t, received)
	require.Equal(t, expected, actual, "default SDK delivery must match standard Zap JSON without erasing native representations")
	require.Contains(t, actual, "nested")
	require.Contains(t, actual, "richVerbose")
	require.Contains(t, actual, "groupCauses")
	require.Empty(t, messages)
}

// TestAlertCoverageCallbackFailuresRejectWholeRecord never admits partial nested data or panic markers.
func TestAlertCoverageCallbackFailuresRejectWholeRecord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field zapcore.Field
	}{
		{"nested object error", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error { return errors.WithStack(enc.AppendObject(fidelityBrokenValue{})) }))},
		{"nested object panic", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error {
			return errors.WithStack(enc.AppendObject(fidelityBrokenValue{panicValue: true}))
		}))},
		{"nested array error", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error { return errors.WithStack(enc.AppendArray(fidelityBrokenValue{})) }))},
		{"nested array panic", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error {
			return errors.WithStack(enc.AppendArray(fidelityBrokenValue{panicValue: true}))
		}))},
		{"array invalid string", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error { enc.AppendString(string([]byte{255})); return nil }))},
		{"array invalid byte string", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error { enc.AppendByteString([]byte{255}); return nil }))},
		{"nested reflected failure", zap.Array("array", alertCoverageArray(func(enc zapcore.ArrayEncoder) error { return errors.WithStack(enc.AppendReflected(make(chan int))) }))},
		{"error callback panic", zap.Error(alertCoveragePanickingError{})},
		{"formatter callback panic", zap.Error(alertCoverageRichError{panicFormat: true})},
		{"group callback panic", zap.Error(alertCoverageGroup{panicMembers: true})},
		{"group member panic", zap.Error(alertCoverageGroup{members: []error{nil, alertCoveragePanickingError{}}})},
		{"malformed native interface", zapcore.Field{Key: "object", Type: zapcore.ObjectMarshalerType, Interface: "not an object marshaler"}},
		{"uninitialized field", zapcore.Field{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alert, messages := captureSecurityAlert(t)
			hook := alert.GetZapHook()
			var err error
			require.NotPanics(t, func() {
				err = hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "rejected operation"}, []zapcore.Field{zap.String("prefix", "must not be sent"), tc.field})
			})
			require.ErrorIs(t, err, ErrAlertEncoding)
			require.NotContains(t, fmt.Sprintf("%+v", err), "SYNTHETIC_CALLBACK_")
			require.Empty(t, alert.senderChan)
			require.NoError(t, hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "after rejected operation"}, []zapcore.Field{zap.String("accepted", "complete positive control")}))
			actual := fidelityAlertFields(t, defaultAlertMessage(t, messages))
			require.Equal(t, map[string]any{"accepted": "complete positive control"}, actual,
				"the next real HTTP delivery is a FIFO barrier against an earlier partial enqueue")
			require.Empty(t, messages)
		})
	}
}

// TestAlertCoverageExpansionBudgetsRejectWholeRecord counts actual escaping and reflected output bytes.
func TestAlertCoverageExpansionBudgetsRejectWholeRecord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field zapcore.Field
	}{
		{"JSON escaping expands within raw text budget", zap.String("body", strings.Repeat("\x00", 64))},
		{"reflected JSON exceeds writer budget", zap.Reflect("reflected", strings.Repeat("x", 257))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alert, messages := captureSecurityAlert(t, WithAlertMaxMessageBytes(256))
			hook := alert.GetZapHook()
			require.ErrorIs(t, hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "oversized representation"}, []zapcore.Field{tc.field}), ErrAlertMessageTooLarge)
			require.Empty(t, alert.senderChan)
			require.NoError(t, hook(zapcore.Entry{Level: zap.ErrorLevel, Message: "after size rejection"}, []zapcore.Field{zap.String("accepted", "whole record")}))
			require.Equal(t, map[string]any{"accepted": "whole record"}, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
			require.Empty(t, messages)
		})
	}
}

// TestAlertCoverageCoreLegacyNamespace preserves explicit filtering across later bound contexts.
func TestAlertCoverageCoreLegacyNamespace(t *testing.T) {
	alert, messages := captureSecurityAlert(t, WithAlertFieldAllowlist("request_id"))
	parent := alert.GetZapCore()
	require.Empty(t, parent.Fields())
	require.NoError(t, parent.Sync())
	stopped := parent.With([]zapcore.Field{zap.String("request_id", "before namespace"), zap.Namespace("private"), zap.String("request_id", "must not replace original")})
	calls := 0
	stopped = stopped.With([]zapcore.Field{zap.Object("ignored", alertCoverageObject(func(zapcore.ObjectEncoder) error { calls++; return errors.New("must not execute") }))})
	require.NoError(t, stopped.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "legacy namespace"}, []zapcore.Field{zap.String("request_id", "call site ignored")}))
	require.Zero(t, calls)
	require.Equal(t, map[string]any{"request_id": "before namespace"}, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
	require.NoError(t, parent.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "parent unchanged"}, []zapcore.Field{zap.String("request_id", "parent value")}))
	require.Equal(t, map[string]any{"request_id": "parent value"}, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
	alert.Close()
	require.Error(t, parent.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "closed"}, nil))
	require.Empty(t, messages)
}

// TestAlertCoverageCoreRejectedAdmission preserves bind errors and rejects malformed call-site fields.
func TestAlertCoverageCoreRejectedAdmission(t *testing.T) {
	alert, messages := captureSecurityAlert(t, WithAlertFieldAllowlist("request_id"), WithAlertMaxMessageBytes(256))
	parent := alert.GetZapCore()
	oversized := []zapcore.Field{zap.String("request_id", strings.Repeat("x", 300))}
	bound := parent.With(oversized)
	require.Empty(t, bound.Fields())
	require.ErrorIs(t, bound.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "invalid bound"}, nil), ErrAlertMessageTooLarge)
	require.ErrorIs(t, parent.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "invalid call"}, oversized), ErrAlertMessageTooLarge)
	require.NoError(t, parent.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "valid after rejection"}, []zapcore.Field{zap.String("request_id", "still usable")}))
	require.Equal(t, map[string]any{"request_id": "still usable"}, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
	faithful, faithfulMessages := captureSecurityAlert(t)
	require.ErrorIs(t, faithful.GetZapCore().Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "bad native value"}, []zapcore.Field{zap.Object("object", fidelityBrokenValue{})}), ErrAlertEncoding)
	require.NoError(t, faithful.GetZapCore().Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "positive native control"}, []zapcore.Field{zap.String("kept", "complete")}))
	require.Equal(t, map[string]any{"kept": "complete"}, fidelityAlertFields(t, defaultAlertMessage(t, faithfulMessages)))
}

// TestAlertCoverageDirectTextRejectsInvalid rejects malformed transport strings before admission.
func TestAlertCoverageDirectTextRejectsInvalid(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	invalid := string([]byte{255})
	for _, tc := range []struct{ name, alertType, token, message string }{
		{"message", "synthetic", "synthetic-auth", invalid},
		{"type", invalid, "synthetic-auth", "chosen original"},
		{"token", "synthetic", invalid, "chosen original"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, alert.SendWithType(tc.alertType, tc.token, tc.message), ErrAlertEncoding)
			require.Empty(t, alert.senderChan)
		})
	}
	require.NoError(t, alert.Send("complete direct positive control"))
	require.Equal(t, "complete direct positive control", defaultAlertMessage(t, messages))
	require.Empty(t, messages)
}
