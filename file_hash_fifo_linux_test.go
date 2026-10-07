//go:build linux

package utils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity64FIFO bounds and reaps the historical blocking-open reproduction.
func TestSecurity64FIFO(t *testing.T) {
	if os.Getenv("GO_UTILS_HASH64_FIFO") == "1" {
		dir := t.TempDir()
		fifo := filepath.Join(dir, "fifo")
		require.NoError(t, syscall.Mkfifo(fifo, 0600))
		require.Error(t, VerifyFileHash(fifo, "sha256:"+strings.Repeat("0", 64)))
		_, err := FileHash(HashTypeSha256, fifo)
		require.Error(t, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestSecurity64FIFO$")
	cmd.Env = append(os.Environ(), "GO_UTILS_HASH64_FIFO=1", "GORACE=atexit_sleep_ms=0")
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "FIFO open did not return before the safety deadline")
	require.NoError(t, err, string(output))
}
