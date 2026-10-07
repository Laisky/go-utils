//go:build linux || darwin

package utils

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runBoundedFIFOCall runs call in a goroutine and fails the test if it does not
// return within two seconds. A blocked open on the FIFO is released by opening
// the read end without blocking, so the goroutine always terminates. It takes
// the test handle, the FIFO path and the operation under test, and returns the
// operation's error.
func runBoundedFIFOCall(t *testing.T, fifo string, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-done
		require.NoError(t, reader.Close())
		t.Fatal("operation blocked on a FIFO destination")
		return nil
	}
}

// TestCopyFileOverwriteFIFO verifies that an overwrite copy rejects a planted FIFO
// promptly instead of blocking on it; regression for issue #46.
func TestCopyFileOverwriteFIFO(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	require.NoError(t, syscall.Mkfifo(dst, 0o600))

	err := runBoundedFIFOCall(t, dst, func() error {
		return CopyFile(fixture.src, dst, Overwrite())
	})
	require.Error(t, err)
	info, err := os.Lstat(dst)
	require.NoError(t, err)
	require.Equal(t, os.ModeNamedPipe, info.Mode().Type())
}
