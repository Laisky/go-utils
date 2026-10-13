package utils

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCopyFileCompatibilityInode preserves mode, inode and existing open handles on overwrite.
func TestCopyFileCompatibilityInode(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("new contents"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0644))
	before, err := os.Stat(dst)
	require.NoError(t, err)
	handle, err := os.Open(dst)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handle.Close()) })
	require.NoError(t, CopyFile(src, dst, Overwrite(), WithFileMode(0600)))
	after, err := os.Stat(dst)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "overwrite must retain the original destination inode")
	if runtime.GOOS != "windows" {
		require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
	}
	got, err := io.ReadAll(handle)
	require.NoError(t, err)
	require.Equal(t, "new contents", string(got))
}

// TestCopyFileCompatibilityHardlinks preserves the original in-place visibility through other hardlinks.
func TestCopyFileCompatibilityHardlinks(t *testing.T) {
	dir := t.TempDir()
	src, dst, link := filepath.Join(dir, "src"), filepath.Join(dir, "dst"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0600))
	if err := os.Link(dst, link); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	require.NoError(t, CopyFile(src, dst, Overwrite()))
	got, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "new", string(got))
}

// TestCopyFileCompatibilitySameSource rejects self-copy before truncation,
// including a second hardlink to the source, while preserving its contents.
func TestCopyFileCompatibilitySameSource(t *testing.T) {
	dir := t.TempDir()
	source, link := filepath.Join(dir, "source"), filepath.Join(dir, "same-inode")
	require.NoError(t, os.WriteFile(source, []byte("retain source"), 0600))
	require.Error(t, CopyFile(source, source, Overwrite()))
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "retain source", string(data))
	if err := os.Link(source, link); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	require.Error(t, CopyFile(source, link, Overwrite()))
	data, err = os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "retain source", string(data))
}
