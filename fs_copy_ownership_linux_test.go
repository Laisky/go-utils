package utils

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCopyFileCompatibilityOwnership verifies that an in-place overwrite leaves
// destination ownership unchanged, including another owner when running as root.
func TestCopyFileCompatibilityOwnership(t *testing.T) {
	dir := t.TempDir()
	source, dst := filepath.Join(dir, "source"), filepath.Join(dir, "dest")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0640))
	if os.Geteuid() == 0 {
		require.NoError(t, os.Chown(dst, 65534, 65534))
	}
	before, err := os.Stat(dst)
	require.NoError(t, err)
	require.NoError(t, CopyFile(source, dst, Overwrite(), WithFileMode(0600)))
	after, err := os.Stat(dst)
	require.NoError(t, err)
	bs, ok := before.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	as, ok := after.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	require.Equal(t, bs.Uid, as.Uid)
	require.Equal(t, bs.Gid, as.Gid)
	require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
}
