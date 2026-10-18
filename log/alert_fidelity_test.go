package log

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// fidelityAlertFields decodes the standard JSON suffix without rounding integer values.
func fidelityAlertFields(t *testing.T, message string) map[string]any {
	t.Helper()
	raw := message[strings.LastIndex(message, "\n")+1:]
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var fields map[string]any
	require.NoError(t, decoder.Decode(&fields))
	return fields
}

// fidelityObject uses the ordinary Zap object contract and counts evaluations.
type fidelityObject struct {
	value string
	calls *atomic.Int64
}

// MarshalLogObject emits a user-chosen nested field exactly once per encoding.
func (o fidelityObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	o.calls.Add(1)
	enc.AddString("body", o.value)
	return nil
}

// fidelityArray exercises the ordinary Zap array contract.
type fidelityArray struct{}

// MarshalLogArray preserves typed values and a nested null.
func (fidelityArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	enc.AppendString("数组\nline")
	enc.AppendInt64(9007199254740993)
	return enc.AppendReflected(nil)
}

// TestAlertFidelityDefaultHookHTTP preserves application-selected messages and fields.
func TestAlertFidelityDefaultHookHTTP(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var calls atomic.Int64
	originalURL := " \thttps://example.test/你好?token=SYNTHETIC_CHOSEN_VALUE#fragment\n"
	message := "request body: <p>chosen 原文</p>\npassword=SYNTHETIC_APPLICATION_TEXT"
	name := "worker/https://example.test/path?chosen=yes"
	fields := []zapcore.Field{
		zap.String("original_url", originalURL),
		zap.String("html", "<body>你好\n<script>chosen()</script></body>"),
		zap.String("body", "body: SYNTHETIC_CHOSEN_BODY"),
		zap.String("markdown", "# 原文\n\nchosen"),
		zap.String("future_unknown_key", "synthetic-auth"),
		zap.Duration("duration", 17*time.Millisecond),
		zap.Int64("exact_integer", 9007199254740993),
		zap.Error(errors.New("read response body: original diagnostic")),
		zap.Reflect("null", nil),
		zap.Binary("binary", []byte{0, 255, 1}),
		zap.Object("object", fidelityObject{"nested original body", &calls}),
		zap.Array("array", fidelityArray{}),
		zap.Namespace("scope"), zap.String("value", "namespaced"),
	}
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, LoggerName: name, Message: message}, fields))
	delivered := defaultAlertMessage(t, messages)
	require.Contains(t, delivered, message)
	require.Contains(t, delivered, name)
	got := fidelityAlertFields(t, delivered)
	require.Equal(t, originalURL, got["original_url"])
	require.Equal(t, "<body>你好\n<script>chosen()</script></body>", got["html"])
	require.Equal(t, "body: SYNTHETIC_CHOSEN_BODY", got["body"])
	require.Equal(t, "# 原文\n\nchosen", got["markdown"])
	require.Equal(t, "synthetic-auth", got["future_unknown_key"])
	require.Equal(t, json.Number("17000000"), got["duration"])
	require.Equal(t, json.Number("9007199254740993"), got["exact_integer"])
	require.Equal(t, "read response body: original diagnostic", got["error"])
	require.Contains(t, got, "null")
	require.Nil(t, got["null"])
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte{0, 255, 1}), got["binary"])
	require.Equal(t, map[string]any{"body": "nested original body"}, got["object"])
	require.Equal(t, []any{"数组\nline", json.Number("9007199254740993"), nil}, got["array"])
	require.Equal(t, map[string]any{"value": "namespaced"}, got["scope"])
	require.EqualValues(t, 1, calls.Load())
}

// TestAlertFidelityConfiguredSizeHTTP accepts complete larger messages only within the explicit budget.
func TestAlertFidelityConfiguredSizeHTTP(t *testing.T) {
	alert, messages := captureSecurityAlert(t, WithAlertMaxMessageBytes(1<<20))
	original := strings.Repeat("x", 20*1024) + "\n完整尾部"
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "large chosen input"}, []zapcore.Field{zap.String("conversion_input", original)}))
	require.Equal(t, original, fidelityAlertFields(t, defaultAlertMessage(t, messages))["conversion_input"])
	require.ErrorIs(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "too large"}, []zapcore.Field{zap.String("conversion_input", strings.Repeat("x", (1<<20)+1))}), ErrAlertMessageTooLarge)
	require.Empty(t, messages)
	require.Empty(t, alert.senderChan)
}

// TestAlertFidelityDefaultBudgetRejectsWholeEntry keeps finite admission observable without altering fields.
func TestAlertFidelityDefaultBudgetRejectsWholeEntry(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	require.ErrorIs(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "large chosen input"}, []zapcore.Field{zap.String("html", strings.Repeat("x", 20*1024))}), ErrAlertMessageTooLarge)
	require.Empty(t, messages)
	require.Empty(t, alert.senderChan)
}

// TestAlertFidelityCoreBoundContext matches standard Zap namespace order and eager snapshots.
func TestAlertFidelityCoreBoundContext(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var actualCalls, oracleCalls atomic.Int64
	var local bytes.Buffer
	core := alert.GetZapCore()
	oracle := zapcore.NewCore(zapcore.NewJSONEncoder(zapcore.EncoderConfig{}), zapcore.AddSync(&local), zap.ErrorLevel)
	actualObject := &fidelityObject{"before mutation", &actualCalls}
	oracleObject := &fidelityObject{"before mutation", &oracleCalls}
	actualBinary, oracleBinary := []byte{0, 255, 1}, []byte{0, 255, 1}
	core = core.With([]zapcore.Field{zap.Namespace("outer"), zap.Object("object", actualObject), zap.Binary("binary", actualBinary)})
	oracle = oracle.With([]zapcore.Field{zap.Namespace("outer"), zap.Object("object", oracleObject), zap.Binary("binary", oracleBinary)})
	core = core.With([]zapcore.Field{zap.Namespace("inner"), zap.String("bound", "bound value")})
	oracle = oracle.With([]zapcore.Field{zap.Namespace("inner"), zap.String("bound", "bound value")})
	require.EqualValues(t, 1, actualCalls.Load(), "With must encode bound objects eagerly")
	require.EqualValues(t, 1, oracleCalls.Load())
	actualObject.value, oracleObject.value = "after mutation", "after mutation"
	actualBinary[0], oracleBinary[0] = 99, 99
	entry := zapcore.Entry{Level: zap.ErrorLevel, LoggerName: "original-core", Message: "namespace operation"}
	for range 2 {
		call := []zapcore.Field{zap.String("call", "call value")}
		require.NoError(t, core.Write(entry, call))
		require.NoError(t, oracle.Write(entry, call))
		got := fidelityAlertFields(t, defaultAlertMessage(t, messages))
		lines := strings.Split(strings.TrimSpace(local.String()), "\n")
		require.Equal(t, fidelityAlertFields(t, lines[len(lines)-1]), got)
		outer, ok := got["outer"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, map[string]any{"body": "before mutation"}, outer["object"])
		require.Equal(t, base64.StdEncoding.EncodeToString([]byte{0, 255, 1}), outer["binary"])
		require.Equal(t, map[string]any{"bound": "bound value", "call": "call value"}, outer["inner"])
	}
	require.EqualValues(t, 1, actualCalls.Load(), "Write must not re-run bound serializers")
	require.EqualValues(t, 1, oracleCalls.Load())
}

// fidelityBrokenValue exercises whole-entry callback failure handling.
type fidelityBrokenValue struct{ panicValue bool }

// MarshalLogObject writes a prefix before failing so partial success cannot pass.
func (v fidelityBrokenValue) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("partial", "must not enqueue")
	if v.panicValue {
		panic("SYNTHETIC_CALLBACK_PANIC")
	}
	return errors.New("SYNTHETIC_CALLBACK_ERROR")
}

// MarshalLogArray writes a prefix before returning a callback error.
func (v fidelityBrokenValue) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	enc.AppendString("must not enqueue")
	if v.panicValue {
		panic("SYNTHETIC_CALLBACK_PANIC")
	}
	return errors.New("SYNTHETIC_CALLBACK_ERROR")
}

// String panics so a synthetic replacement cannot be mistaken for faithful data.
func (fidelityBrokenValue) String() string { panic("SYNTHETIC_CALLBACK_PANIC") }

// TestAlertFidelityEncodingFailuresRejectWholeEntry never queues partial callback output.
func TestAlertFidelityEncodingFailuresRejectWholeEntry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field zapcore.Field
	}{
		{"object error", zap.Object("object", fidelityBrokenValue{})},
		{"object panic", zap.Object("object", fidelityBrokenValue{true})},
		{"array error", zap.Array("array", fidelityBrokenValue{})},
		{"inline error", zap.Inline(fidelityBrokenValue{})},
		{"stringer panic", zap.Stringer("stringer", fidelityBrokenValue{})},
		{"unsupported reflected", zap.Reflect("unsupported", make(chan int))},
		{"unknown type", zapcore.Field{Key: "unknown", Type: zapcore.FieldType(255)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alert, messages := captureSecurityAlert(t)
			var err error
			require.NotPanics(t, func() {
				err = alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "operation"}, []zapcore.Field{zap.String("prefix", "original"), tc.field})
			})
			require.ErrorIs(t, err, ErrAlertEncoding)
			require.NotContains(t, err.Error(), "SYNTHETIC_")
			require.Empty(t, messages)
			require.Empty(t, alert.senderChan)
		})
	}
}

// TestAlertFidelityInvalidUTF8RejectsWholeEntry preserves binary support without silent text repair.
func TestAlertFidelityInvalidUTF8RejectsWholeEntry(t *testing.T) {
	invalid := string([]byte{255})
	for _, tc := range []struct {
		name   string
		entry  zapcore.Entry
		fields []zapcore.Field
	}{
		{"message", zapcore.Entry{Level: zap.ErrorLevel, Message: invalid}, nil},
		{"logger name", zapcore.Entry{Level: zap.ErrorLevel, LoggerName: invalid}, nil},
		{"string field", zapcore.Entry{Level: zap.ErrorLevel}, []zapcore.Field{zap.String("text", invalid)}},
		{"field key", zapcore.Entry{Level: zap.ErrorLevel}, []zapcore.Field{zap.String(invalid, "text")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alert, messages := captureSecurityAlert(t)
			require.ErrorIs(t, alert.GetZapHook()(tc.entry, tc.fields), ErrAlertEncoding)
			require.Empty(t, messages)
			require.Empty(t, alert.senderChan)
		})
	}
}

// TestAlertFidelityCoreTeePreservesLocalSink exercises the production core attachment path.
func TestAlertFidelityCoreTeePreservesLocalSink(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var local bytes.Buffer
	localCore := zapcore.NewCore(zapcore.NewJSONEncoder(zapcore.EncoderConfig{}), zapcore.AddSync(&local), zap.DebugLevel)
	logger := zap.New(zapcore.NewTee(localCore, alert.GetZapCore())).Named("original-name")
	logger.With(zap.Namespace("worker"), zap.String("bound", "value")).Error("original message", zap.String("url", "https://example.test/chosen?q=yes"))
	got := fidelityAlertFields(t, defaultAlertMessage(t, messages))
	require.Equal(t, fidelityAlertFields(t, strings.TrimSpace(local.String())), got)
}
