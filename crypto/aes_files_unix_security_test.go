//go:build linux || darwin || freebsd || netbsd || openbsd

package crypto

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAESFilesInDirSecurity_SkipsFIFO checks that a FIFO in the directory is
// skipped without blocking. Regression for issue #65.
func TestAESFilesInDirSecurity_SkipsFIFO(t *testing.T) {
	t.Parallel()

	dir := writeFilesTestDir(t, map[string]string{"a.txt": "plain"})
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe.txt"), 0o600))

	require.NoError(t, AESEncryptFilesInDir(dir, filesTestKey))
	_, err := os.Lstat(filepath.Join(dir, "pipe.txt.enc"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "a.txt.enc"))
	require.NoError(t, err)
}
