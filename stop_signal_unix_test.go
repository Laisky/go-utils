//go:build unix

package utils

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStopSignal retains real Unix signal delivery in a bounded child process.
// The process-wide one-handler restriction cannot leak between test repetitions,
// and Unix-only syscalls no longer prevent Windows path regressions from building.
func TestStopSignal(t *testing.T) {
	if os.Getenv("GO_UTILS_STOP_SIGNAL_CHILD") == "1" {
		stopCh := StopSignal(WithStopSignalCloseSignals(os.Interrupt, syscall.SIGTERM))
		select {
		case <-stopCh:
			t.Fatal("should not be closed before signal delivery")
		default:
		}
		require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))
		select {
		case _, ok := <-stopCh:
			require.False(t, ok)
		case <-time.After(5 * time.Second):
			t.Fatal("stop channel did not close after signal delivery")
		}
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestStopSignal$", "-test.timeout=8s")
	child.Env = append(os.Environ(), "GO_UTILS_STOP_SIGNAL_CHILD=1")
	output, err := child.CombinedOutput()
	require.NoError(t, ctx.Err(), "signal test child exceeded deadline")
	require.NoError(t, err, string(output))
}
