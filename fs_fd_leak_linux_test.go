//go:build linux

package utils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// openFDCount returns the number of open file descriptors of this process.
func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	return len(entries)
}

// TestFileDigestHelpersCloseDescriptors verifies that FileMD5 and FileSHA1
// close the file they open. FileSHA1 backs WatchFileChanging, so a leak there
// grows by one descriptor per detected change in long-running watchers.
func TestFileDigestHelpersCloseDescriptors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("digest me"), 0o600))

	const calls = 200
	before := openFDCount(t)
	for range calls {
		_, err := FileMD5(path)
		require.NoError(t, err)
		_, err = FileSHA1(path)
		require.NoError(t, err)
	}
	require.Less(t, openFDCount(t)-before, 20, "file digest helpers leaked descriptors")
}

// TestFlockFailedLockReleasesDescriptor verifies that a Lock call that fails
// because another process holds the lock closes the descriptor it opened.
func TestFlockFailedLockReleasesDescriptor(t *testing.T) {
	lockPath := os.Getenv("GO_UTILS_FLOCK_HOLDER")
	if lockPath != "" {
		// Child: hold the lock until the parent kills the process.
		lock, err := NewFlock(lockPath)
		require.NoError(t, err)
		require.NoError(t, lock.Lock())
		require.NoError(t, os.WriteFile(lockPath+".ready", nil, 0o600))
		time.Sleep(time.Minute)
		return
	}

	lockPath = filepath.Join(t.TempDir(), "held.lock")
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	holder := exec.CommandContext(ctx, executable, "-test.run=^TestFlockFailedLockReleasesDescriptor$")
	holder.Env = append(os.Environ(), "GO_UTILS_FLOCK_HOLDER="+lockPath)
	require.NoError(t, holder.Start())
	t.Cleanup(func() {
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(lockPath + ".ready")
		return statErr == nil
	}, 20*time.Second, 10*time.Millisecond, "lock holder never became ready")

	before := openFDCount(t)
	for range 50 {
		lock, err := NewFlock(lockPath)
		require.NoError(t, err)
		require.Error(t, lock.Lock(), "lock held by another process must not be acquired")
	}
	require.Less(t, openFDCount(t)-before, 10, "failed Lock calls leaked descriptors")
}
