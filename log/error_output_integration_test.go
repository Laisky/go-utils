package log

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

// security77FailingSink produces one synthetic internal Zap error without any external I/O.
type security77FailingSink struct{}

// Write rejects a synthetic application entry so Zap exercises its internal diagnostic routing.
func (security77FailingSink) Write([]byte) (int, error) {
	return 0, errors.New("SYNTHETIC_SINK_FAILURE")
}

// Sync has no buffered data or work to flush.
func (security77FailingSink) Sync() error { return nil }

// Close has no owned resources to release.
func (security77FailingSink) Close() error { return nil }

// TestSecurity77ErrorSinkRouting captures subprocess stderr while a real Zap sink fails.
// It verifies private routing, explicitly selected stderr, and deliberately disabled diagnostics.
func TestSecurity77ErrorSinkRouting(t *testing.T) {
	if mode := os.Getenv("GO_UTILS_SECURITY77_CHILD"); mode != "" {
		require.NoError(t, zap.RegisterSink("security77fail", func(*url.URL) (zap.Sink, error) {
			return security77FailingSink{}, nil
		}))
		paths := []string{os.Getenv("GO_UTILS_SECURITY77_DEST")}
		switch mode {
		case "private":
		case "console":
			paths = append(paths, "stderr")
		case "disabled":
			paths = nil
		default:
			t.Fatal("unknown child mode")
		}
		logger, err := New(WithOutputPaths([]string{"security77fail://sink"}), WithErrorOutputPaths(paths))
		require.NoError(t, err)
		logger.Info("benign synthetic application entry")
		require.NoError(t, logger.Sync())
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, mode := range []string{"private", "console", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			destination := filepath.Join(t.TempDir(), "internal.log")
			command := exec.CommandContext(ctx, executable, "-test.run=^TestSecurity77ErrorSinkRouting$", "-test.count=1", "-test.timeout=5s")
			command.Env = append(os.Environ(), "GO_UTILS_SECURITY77_CHILD="+mode, "GO_UTILS_SECURITY77_DEST="+destination)
			var console bytes.Buffer
			command.Stderr = &console
			require.NoError(t, command.Run(), console.String())
			if mode == "console" {
				require.Contains(t, console.String(), "SYNTHETIC_SINK_FAILURE")
			} else {
				require.NotContains(t, console.String(), "SYNTHETIC_SINK_FAILURE")
			}
			if mode == "disabled" {
				_, err := os.Stat(destination)
				require.True(t, os.IsNotExist(err))
				return
			}
			body, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Contains(t, string(body), "SYNTHETIC_SINK_FAILURE")
		})
	}
}
