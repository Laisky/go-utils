package log

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// TestLoggerEncodingOptions verifies that the last encoding option controls both the format and level encoder.
func TestLoggerEncodingOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		formats []Encoding
		json    bool
	}{
		{"json", []Encoding{EncodingJSON}, true},
		{"console", []Encoding{EncodingConsole}, false},
		{"json_to_console", []Encoding{EncodingJSON, EncodingConsole}, false},
		{"console_to_json", []Encoding{EncodingConsole, EncodingJSON}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "encoding.log")
			opts := []Option{WithOutputPaths([]string{path}), WithName("compat")}
			for _, format := range tc.formats {
				opts = append(opts, WithEncoding(format))
			}
			logger, err := New(opts...)
			require.NoError(t, err)
			logger.Info("encoding", zap.String("field", "value"))
			require.NoError(t, logger.Sync())
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			if tc.json {
				var entry map[string]any
				require.NoError(t, json.Unmarshal(raw, &entry))
				require.Equal(t, "INFO", entry["level"])
				require.Equal(t, "encoding", entry["message"])
				require.Equal(t, "compat", entry["logger"])
				require.Equal(t, "value", entry["field"])
				require.NotContains(t, string(raw), `\u001b`)
			} else {
				require.False(t, json.Valid(raw), "console selection must replace JSON")
				require.Contains(t, string(raw), "INFO")
				require.Contains(t, string(raw), "\x1b[")
				require.Contains(t, string(raw), "encoding")
			}
		})
	}
}

// TestLoggerSamplingBoundaries verifies suppression and certainty for all three sampled logging methods.
func TestLoggerSamplingBoundaries(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"debug", "info", "warn"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(io.Discard), zap.DebugLevel)
			logger := &LoggerT{Logger: zap.New(core, zap.Hooks(func(zapcore.Entry) error {
				calls++
				return nil
			}))}
			methods := map[string]func(int, string, ...zapcore.Field){
				"debug": logger.DebugSample, "info": logger.InfoSample, "warn": logger.WarnSample,
			}
			emit := methods[name]
			t.Run("zero", func(t *testing.T) {
				for range 50000 {
					emit(-1, "negative")
					emit(0, "zero")
				}
				require.Zero(t, calls, "zero and negative sample rates must never emit")
			})
			t.Run("certain", func(t *testing.T) {
				calls = 0
				for range 10 {
					emit(SampleRateDenominator, "certain")
					emit(SampleRateDenominator+1, "above")
				}
				require.Equal(t, 20, calls)
			})
			t.Run("fractional", func(t *testing.T) {
				calls = 0
				for range 50000 {
					emit(SampleRateDenominator-1, "below")
				}
				// The old inclusive bound always emitted at 999. Missing every
				// rejection with the correct 999/1000 rate has probability < 2e-22.
				require.Less(t, calls, 50000, "999/1000 must allow rejection")
				require.Greater(t, calls, 49000)
			})
		})
	}
}

// TestLoggerJSONFields verifies child isolation, error encoding, and the shared dynamic level through the fork.
func TestLoggerJSONFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "fields.log")
	logger, err := New(WithName("compat"), WithEncoding(EncodingJSON), WithLevel(LevelInfo),
		WithOutputPaths([]string{path}))
	require.NoError(t, err)
	child := logger.Named("child").With(zap.String("bound", "child"))
	logger.Info("parent", zap.Int("count", 1))
	child.Info("child", zap.Bool("ready", true), zap.ByteString("bytes", []byte("text")))
	require.NoError(t, child.ChangeLevel(LevelError))
	require.Equal(t, LevelError, logger.Level())
	logger.Info("suppressed")
	child.Error("error", zap.NamedError("failure", io.EOF))
	require.NoError(t, logger.Sync())
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 3)
	entries := make([]map[string]any, len(lines))
	for i, line := range lines {
		require.NoError(t, json.Unmarshal([]byte(line), &entries[i]))
	}
	require.NotContains(t, entries[0], "bound")
	require.Equal(t, float64(1), entries[0]["count"])
	require.Equal(t, "compat.child", entries[1]["logger"])
	require.Equal(t, "child", entries[1]["bound"])
	require.Equal(t, true, entries[1]["ready"])
	require.Equal(t, "text", entries[1]["bytes"])
	require.Equal(t, "EOF", entries[2]["failure"])
	for _, entry := range entries {
		require.Contains(t, entry, "caller")
		require.Contains(t, entry, "ts")
	}
}
