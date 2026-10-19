package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// fidelityInlineText emits a single large ordinary scalar through Inline.
type fidelityInlineText string

// MarshalLogObject writes the selected value through the existing encoder.
func (v fidelityInlineText) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("inline_body", string(v))
	return nil
}

// TestAlertFidelityInlineBudgetCountsOnce accepts complete JSON smaller than its configured budget.
func TestAlertFidelityInlineBudgetCountsOnce(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	value := strings.Repeat("x", 10*1024)
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "inline"}, []zapcore.Field{zap.Inline(fidelityInlineText(value))}))
	require.Equal(t, value, fidelityAlertFields(t, defaultAlertMessage(t, messages))["inline_body"])
}

// TestAlertFidelityDuplicateKeysRemainRaw preserves Zap's ordered duplicate-key representation.
func TestAlertFidelityDuplicateKeysRemainRaw(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	core := alert.GetZapCore().With([]zapcore.Field{zap.String("duplicate", "bound")})
	require.NoError(t, core.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "duplicates"}, []zapcore.Field{zap.String("duplicate", "call")}))
	delivered := defaultAlertMessage(t, messages)
	require.Contains(t, delivered, `{"duplicate":"bound","duplicate":"call"}`)
}

// fidelityEmbedded carries exported JSON fields through an unexported anonymous type.
type fidelityEmbedded struct {
	Visible string
	Shadow  string
}

// fidelityZeroString deliberately omits its value through the standard Go 1.26 JSON contract.
type fidelityZeroString string

// IsZero selects omission independently of the underlying string contents.
func (fidelityZeroString) IsZero() bool { return true }

// fidelityJSONProjection is an explicit application-selected JSON marshaler.
type fidelityJSONProjection struct{ calls *atomic.Int64 }

// MarshalJSON returns the declared JSON projection once per standard JSON encoding.
func (v fidelityJSONProjection) MarshalJSON() ([]byte, error) {
	v.calls.Add(1)
	return []byte(`{"declared":"JSON 原文","precise":9007199254740993}`), nil
}

// fidelityTextProjection exposes a selected text projection rather than its internal bytes.
type fidelityTextProjection struct {
	internal string
	calls    *atomic.Int64
}

// MarshalText selects the same standard JSON representation as the local Zap encoder.
func (v fidelityTextProjection) MarshalText() ([]byte, error) {
	v.calls.Add(1)
	return []byte("text 原文\nprojection"), nil
}

// fidelityStandardFields encodes an oracle with the pinned standard Zap JSON encoder.
func fidelityStandardFields(t *testing.T, fields []zapcore.Field) map[string]any {
	t.Helper()
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
	buffer, err := encoder.EncodeEntry(zapcore.Entry{}, fields)
	require.NoError(t, err)
	require.NotNil(t, buffer)
	defer buffer.Free()
	return fidelityAlertFields(t, strings.TrimSpace(buffer.String()))
}

// TestAlertFidelityReflectUsesStandardJSONProjection matches standard omitted, embedded and custom fields.
func TestAlertFidelityReflectUsesStandardJSONProjection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value func(*atomic.Int64) any
	}{
		{"visibility and omitzero", func(*atomic.Int64) any {
			return struct {
				fidelityEmbedded
				Shadow  string
				Omitted string             `json:"-"`
				Zero    fidelityZeroString `json:"zero,omitzero"`
			}{
				fidelityEmbedded: fidelityEmbedded{Visible: "embedded 原文", Shadow: string([]byte{255})},
				Shadow:           "visible shadow",
				Omitted:          string([]byte{255}),
				Zero:             fidelityZeroString(string([]byte{255})),
			}
		}},
		{"custom JSON once", func(calls *atomic.Int64) any { return fidelityJSONProjection{calls} }},
		{"custom text once", func(calls *atomic.Int64) any {
			return fidelityTextProjection{internal: string([]byte{255}), calls: calls}
		}},
		{"standard invalid text normalization", func(*atomic.Int64) any {
			return map[string]any{"chosen_reflect": string([]byte{255}), "nil": nil, "empty": []any{}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alert, messages := captureSecurityAlert(t)
			var actualCalls, oracleCalls atomic.Int64
			expected := fidelityStandardFields(t, []zapcore.Field{zap.Reflect("reflected", tc.value(&oracleCalls))})
			require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "declared JSON projection"}, []zapcore.Field{zap.Reflect("reflected", tc.value(&actualCalls))}))
			require.Equal(t, expected, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
			require.Equal(t, oracleCalls.Load(), actualCalls.Load())
			if strings.Contains(tc.name, "once") {
				require.EqualValues(t, 1, actualCalls.Load())
			}
			if tc.name == "visibility and omitzero" {
				value, ok := expected["reflected"].(map[string]any)
				require.True(t, ok)
				require.NotContains(t, value, "zero")
				require.NotContains(t, value, "Omitted")
				require.Equal(t, "visible shadow", value["Shadow"])
				require.Equal(t, "embedded 原文", value["Visible"])
			}
		})
	}
}

// fidelityNilValue follows Zap's typed-nil formatter convention.
type fidelityNilValue struct{ value string }

// Error deliberately dereferences its nil receiver so standard Zap emits its nil marker.
func (v *fidelityNilValue) Error() string { return v.value }

// String deliberately dereferences its nil receiver so standard Zap emits its nil marker.
func (v *fidelityNilValue) String() string { return v.value }

// TestAlertFidelityTypedNilMatchesStandardZap preserves legitimate typed-nil values.
func TestAlertFidelityTypedNilMatchesStandardZap(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var value *fidelityNilValue
	fields := []zapcore.Field{zap.Error(value), zap.Stringer("stringer", value)}
	expected := fidelityStandardFields(t, fields)
	require.Equal(t, "<nil>", expected["error"])
	require.Equal(t, "<nil>", expected["stringer"])
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "typed nil"}, fields))
	require.Equal(t, expected, fidelityAlertFields(t, defaultAlertMessage(t, messages)))
}

// TestAlertFidelityByteStringInvalidRejects keeps text rejection separate from supported Binary.
func TestAlertFidelityByteStringInvalidRejects(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	require.ErrorIs(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "text encoding"}, []zapcore.Field{zap.ByteString("text", []byte{255})}), ErrAlertEncoding)
	require.Empty(t, alert.senderChan)
	require.Empty(t, messages)
}

// TestAlertFidelityBoundEncodingFailureRejects preserves errors across immutable child cores.
func TestAlertFidelityBoundEncodingFailureRejects(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	parent := alert.GetZapCore()
	broken := parent.With([]zapcore.Field{zap.Object("broken", fidelityBrokenValue{})})
	child := broken.With([]zapcore.Field{zap.String("later", "must not rescue encoding")})
	require.ErrorIs(t, child.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "failed bound value"}, nil), ErrAlertEncoding)
	require.Empty(t, alert.senderChan)
	require.Empty(t, messages)
	require.NoError(t, parent.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "parent unaffected"}, []zapcore.Field{zap.String("kept", "parent value")}))
	require.Equal(t, "parent value", fidelityAlertFields(t, defaultAlertMessage(t, messages))["kept"])
}

// TestAlertFidelityCoreConcurrentSnapshots keeps sibling namespaces and buffers independent.
func TestAlertFidelityCoreConcurrentSnapshots(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var calls atomic.Int64
	base := alert.GetZapCore().With([]zapcore.Field{zap.Namespace("worker"), zap.Object("bound", fidelityObject{"immutable", &calls})})
	require.EqualValues(t, 1, calls.Load())
	const count = 4
	var wait sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			child := base.With([]zapcore.Field{zap.Int("child", i)})
			errs <- child.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "concurrent snapshot"}, nil)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	seen := make(map[json.Number]bool)
	for range count {
		fields := fidelityAlertFields(t, defaultAlertMessage(t, messages))
		worker, ok := fields["worker"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, map[string]any{"body": "immutable"}, worker["bound"])
		index, ok := worker["child"].(json.Number)
		require.True(t, ok)
		seen[index] = true
	}
	require.Len(t, seen, count)
	require.EqualValues(t, 1, calls.Load())
}

// TestAlertFidelityFieldsSnapshot replays immutable bound JSON without retaining application callbacks.
func TestAlertFidelityFieldsSnapshot(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	var calls atomic.Int64
	object := &fidelityObject{"before mutation", &calls}
	binary := []byte{0, 255, 1}
	reflected := map[string]any{"empty": []any{}, "nil": nil}
	core := alert.GetZapCore().With([]zapcore.Field{
		zap.String("duplicate", "first"), zap.String("duplicate", "second"),
		zap.Int64("exact_integer", 9007199254740993),
		zap.Binary("binary", binary), zap.Object("object", object),
		zap.Reflect("reflected", reflected),
		zap.Namespace("nested"), zap.String("chosen", "原文\ntext"), zap.Reflect("nil", nil),
	})
	require.EqualValues(t, 1, calls.Load())
	object.value = "after mutation"
	binary[0] = 99
	reflected["empty"] = "after mutation"
	first := core.Fields()
	require.Len(t, first, 1)
	require.Equal(t, zapcore.InlineMarshalerType, first[0].Type)
	first[0] = zap.String("tampered_returned_slice", "must not alter core")
	fields := core.Fields()
	require.Len(t, fields, 1)
	require.Equal(t, zapcore.InlineMarshalerType, fields[0].Type)
	require.NoError(t, core.Write(zapcore.Entry{Level: zap.ErrorLevel, Message: "snapshot reference"}, nil))
	delivered := defaultAlertMessage(t, messages)
	expected := delivered[strings.LastIndex(delivered, "\n")+1:]
	require.Contains(t, expected, `"duplicate":"first","duplicate":"second"`)
	require.Contains(t, expected, `"exact_integer":9007199254740993`)
	actual := fidelityAlertFields(t, expected)
	require.Equal(t, map[string]any{"body": "before mutation"}, actual["object"])
	require.Equal(t, map[string]any{"empty": []any{}, "nil": nil}, actual["reflected"])
	require.Equal(t, map[string]any{"chosen": "原文\ntext", "nil": nil}, actual["nested"])
	require.Equal(t, "AP8B", actual["binary"])

	type replayResult struct {
		raw string
		err error
	}
	const count = 4
	results := make(chan replayResult, count)
	var wait sync.WaitGroup
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
			buffer, err := encoder.EncodeEntry(zapcore.Entry{}, fields)
			result := replayResult{err: err}
			if buffer != nil {
				result.raw = strings.TrimSpace(buffer.String())
				buffer.Free()
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	for result := range results {
		require.NoError(t, result.err)
		require.Equal(t, expected, result.raw)
	}
	require.EqualValues(t, 1, calls.Load(), "Fields and every replay must use encoded snapshots")
	again := fidelityStandardFields(t, core.Fields())
	require.Equal(t, actual, again)
	require.EqualValues(t, 1, calls.Load())
}

// TestAlertFidelityCoreCheckControlsAdmission preserves the standard Core Check/Write division.
func TestAlertFidelityCoreCheckControlsAdmission(t *testing.T) {
	alert, messages := captureSecurityAlert(t)
	core := alert.GetZapCore()
	var local bytes.Buffer
	oracle := zapcore.NewCore(zapcore.NewJSONEncoder(zapcore.EncoderConfig{}), zapcore.AddSync(&local), zap.ErrorLevel)
	below := zapcore.Entry{Level: zap.InfoLevel, Message: "explicit already checked write"}
	require.False(t, core.Enabled(below.Level))
	require.False(t, oracle.Enabled(below.Level))
	require.Nil(t, core.Check(below, nil))
	require.Nil(t, oracle.Check(below, nil))
	require.True(t, core.Enabled(zap.ErrorLevel))
	require.NotNil(t, core.Check(zapcore.Entry{Level: zap.ErrorLevel}, nil))
	require.Empty(t, alert.senderChan, "Check registers a writer without admitting a payload")
	require.Empty(t, messages)
	fields := []zapcore.Field{zap.String("chosen", "原文\nlower-level explicit Write")}
	require.NoError(t, oracle.Write(below, fields))
	require.NoError(t, core.Write(below, fields), "Write must not repeat the level decision already owned by Check")
	delivered := defaultAlertMessage(t, messages)
	require.Contains(t, delivered, "level: `info`")
	require.Equal(t, fidelityAlertFields(t, strings.TrimSpace(local.String())), fidelityAlertFields(t, delivered))
}
