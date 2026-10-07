//go:build linux

package fileguard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFIFOReplacement runs a substituted FIFO open in a bounded, reaped child.
func TestFIFOReplacement(t *testing.T) {
	if os.Getenv("GO_UTILS_FILEGUARD_FIFO") == "1" {
		dir := t.TempDir()
		p := filepath.Join(dir, "entry")
		require.NoError(t, os.WriteFile(p, []byte("data"), 0600))
		root, err := OpenRoot(dir)
		require.NoError(t, err)
		defer root.Close()
		info, err := root.Lstat("entry")
		require.NoError(t, err)
		require.NoError(t, os.Remove(p))
		require.NoError(t, syscall.Mkfifo(p, 0600))
		f, err := OpenRegular(root, "entry", info)
		require.Error(t, err)
		require.Nil(t, f)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exe, err := os.Executable()
	require.NoError(t, err)
	child := exec.CommandContext(ctx, exe, "-test.run=^TestFIFOReplacement$")
	child.Env = append(os.Environ(), "GO_UTILS_FILEGUARD_FIFO=1", "GORACE=atexit_sleep_ms=0")
	out, err := child.CombinedOutput()
	require.NoError(t, ctx.Err(), "FIFO replacement blocked open")
	require.NoError(t, err, string(out))
}
