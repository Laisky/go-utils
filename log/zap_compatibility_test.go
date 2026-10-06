package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestZapForkHooksFieldsAndFilter checks context isolation, field hooks, filtering, and raised levels through the wrapper.
func TestZapForkHooksFieldsAndFilter(t *testing.T) {
	t.Parallel()
	for _, lazy := range []bool{false, true} {
		t.Run(map[bool]string{false: "eager", true: "lazy"}[lazy], func(t *testing.T) {
			var observed *observer.ObservedLogs
			var hooked [][]zapcore.Field
			var filtered [][]zapcore.Field
			logger, err := New(WithLevel(LevelDebug), WithOutputPaths([]string{}), WithZapOptions(
				zap.WrapCore(func(core zapcore.Core) zapcore.Core {
					var capture zapcore.Core
					capture, observed = observer.New(core)
					return capture
				}),
				zap.HooksWithFields(func(_ zapcore.Entry, fields []zapcore.Field) error {
					hooked = append(hooked, append([]zapcore.Field(nil), fields...))
					return nil
				}),
				zap.Filter(func(entry zapcore.Entry, fields []zapcore.Field) bool {
					filtered = append(filtered, append([]zapcore.Field(nil), fields...))
					return entry.Message != "blocked"
				}),
			))
			require.NoError(t, err)
			parent := logger.With(zap.String("parent", "one"))
			child := parent.Zap().With(zap.String("child", "two"))
			if lazy {
				child = parent.Zap().WithLazy(zap.String("child", "two"))
			}
			child.Info("allowed", zap.Int("call", 3))
			child.Info("blocked", zap.Int("call", 4))
			child.WithOptions(zap.IncreaseLevel(zap.WarnLevel)).Info("below")
			require.Len(t, observed.All(), 1)
			require.Equal(t, map[string]any{"parent": "one", "child": "two", "call": int64(3)},
				observed.All()[0].ContextMap())
			require.Equal(t, []zapcore.Field{zap.String("parent", "one")}, parent.Core().Fields())
			require.Empty(t, logger.Core().Fields())
			require.Equal(t, []zapcore.Field{zap.String("parent", "one"), zap.String("child", "two")}, child.Core().Fields())
			require.Equal(t, [][]zapcore.Field{{zap.Int("call", 3), zap.String("parent", "one"), zap.String("child", "two")}}, hooked)
			require.Equal(t, [][]zapcore.Field{
				{zap.String("parent", "one"), zap.String("child", "two")},
				{zap.String("parent", "one"), zap.String("child", "two")},
			}, filtered)
		})
	}
}

// compatibilityObject provides a mutable object and marshal counter for eager and lazy context tests.
type compatibilityObject struct {
	value int
	calls int
}

// MarshalLogObject records object evaluation and encodes the current value into the supplied encoder.
func (o *compatibilityObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	o.calls++
	enc.AddInt("value", o.value)
	return nil
}

// TestZapForkLazyEncoding verifies that lazy context captures objects at first use and remains isolated from its parent.
func TestZapForkLazyEncoding(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lazy.log")
	logger, err := New(WithEncoding(EncodingJSON), WithOutputPaths([]string{path}))
	require.NoError(t, err)
	eagerObject := &compatibilityObject{value: 1}
	lazyObject := &compatibilityObject{value: 1}
	eager := logger.With(zap.Object("object", eagerObject))
	lazy := logger.Zap().WithLazy(zap.Object("object", lazyObject))
	require.Equal(t, 1, eagerObject.calls)
	require.Zero(t, lazyObject.calls)
	eagerObject.value = 2
	lazyObject.value = 2
	eager.Info("eager")
	lazy.Info("lazy")
	lazyObject.value = 3
	lazy.Info("cached")
	logger.Info("parent")
	require.Equal(t, 1, eagerObject.calls)
	require.Equal(t, 1, lazyObject.calls)
	require.NoError(t, logger.Sync())
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 4)
	entries := make([]map[string]any, 4)
	for i, line := range lines {
		require.NoError(t, json.Unmarshal([]byte(line), &entries[i]))
	}
	require.Equal(t, map[string]any{"value": float64(1)}, entries[0]["object"])
	require.Equal(t, map[string]any{"value": float64(2)}, entries[1]["object"])
	require.Equal(t, map[string]any{"value": float64(2)}, entries[2]["object"])
	require.NotContains(t, entries[3], "object")
}

// TestZapForkSampling checks deterministic core sampling decisions and field-aware hooks through sampled child contexts.
func TestZapForkSampling(t *testing.T) {
	t.Parallel()
	var observed *observer.ObservedLogs
	var decisions []zapcore.SamplingDecision
	hooks := 0
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			capture, entries := observer.New(core)
			observed = entries
			return zapcore.NewSamplerWithOptions(capture, time.Hour, 2, 3,
				zapcore.SamplerHook(func(_ zapcore.Entry, decision zapcore.SamplingDecision) {
					decisions = append(decisions, decision)
				}))
		}),
		zap.HooksWithFields(func(_ zapcore.Entry, fields []zapcore.Field) error {
			hooks++
			require.Contains(t, fields, zap.String("bound", "context"))
			return nil
		}),
	))
	require.NoError(t, err)
	child := logger.With(zap.String("bound", "context"))
	for i := range 10 {
		child.Info("repeated", zap.Int("sequence", i))
	}
	require.Len(t, decisions, 10)
	require.Equal(t, 4, hooks)
	entries := observed.All()
	require.Len(t, entries, 4)
	for i, index := range []int64{0, 1, 4, 7} {
		require.Equal(t, index, entries[i].ContextMap()["sequence"])
		require.Equal(t, "context", entries[i].ContextMap()["bound"])
	}
	for i, decision := range decisions {
		if i == 0 || i == 1 || i == 4 || i == 7 {
			require.Equal(t, zapcore.LogSampled, decision)
		} else {
			require.Equal(t, zapcore.LogDropped, decision)
		}
	}
}
