package log

import (
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestZapForkMultipleFieldHooks verifies that each registered callback receives context exactly once.
func TestZapForkMultipleFieldHooks(t *testing.T) {
	core, _ := observer.New(zap.DebugLevel)
	var got [][]zapcore.Field
	hook := func(_ zapcore.Entry, fields []zapcore.Field) error {
		got = append(got, append([]zapcore.Field(nil), fields...))
		return nil
	}
	zap.New(core, zap.Fields(zap.String("bound", "one")), zap.HooksWithFields(hook, hook)).
		Info("entry", zap.Int("call", 1))
	expected := []zapcore.Field{zap.Int("call", 1), zap.String("bound", "one")}
	require.Equal(t, [][]zapcore.Field{expected, expected}, got)
}

// TestZapForkTeeFilterPreservesOtherCores verifies that rejecting one tee sink does not erase an accepted sibling sink.
func TestZapForkTeeFilterPreservesOtherCores(t *testing.T) {
	for _, rejectingFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejecting_last", true: "rejecting_first"}[rejectingFirst], func(t *testing.T) {
			a, entriesA := observer.New(zap.InfoLevel)
			b, entriesB := observer.New(zap.InfoLevel)
			rejecting := zapcore.RegisterFilter(b, func(zapcore.Entry, []zapcore.Field) bool { return false })
			cores := []zapcore.Core{a, rejecting}
			if rejectingFirst {
				cores = []zapcore.Core{rejecting, a}
			}
			zap.New(zapcore.NewTee(cores...)).Info("keep-a")
			require.Equal(t, 1, entriesA.Len())
			require.Zero(t, entriesB.Len())
		})
	}
}
