//go:build linux || darwin

package log

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRotationRejectsFIFOAtDailyFile verifies that a FIFO planted at the daily
// filename is rejected promptly instead of blocking the logger; regression for issue #46.
// A blocked open is released by opening the read end without blocking, so the
// test goroutine always terminates and the temporary directory can be removed.
func TestRotationRejectsFIFOAtDailyFile(t *testing.T) {
	dir, _ := rotationLinkFixture(t)
	fifo := filepath.Join(dir, "app-20250101.log")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	writer := newFixedRotationWriter(t, dir)

	done := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("log line\n"))
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-done
		require.NoError(t, reader.Close())
		t.Fatal("rotation open blocked on a FIFO")
	}
}
