//go:build linux

package utils

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity50DirectChildReaped verifies that an overlong writer is no longer a child or live process.
func TestSecurity50DirectChildReaped(t *testing.T) {
	exe, args, env := security50Child(t, "stdout")
	pidFile := filepath.Join(t.TempDir(), "pid")
	env = append(env, "GO_UTILS_CMD50_PID="+pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := RunCMD2(ctx, exe, args, env, func(string) {}, func(string) {})
	require.ErrorIs(t, err, ErrCMDLineLimit)
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
	var status syscall.WaitStatus
	_, err = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
	require.ErrorIs(t, err, syscall.ECHILD)
}
