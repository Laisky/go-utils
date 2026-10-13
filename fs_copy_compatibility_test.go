package utils

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCopyFileCompatibilityInode preserves the inode, creation-mode semantics and open handles of an overwritten regular file.
func TestCopyFileCompatibilityInode(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("new content"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old content is longer"), 0600))
	before, err := os.Stat(dst)
	require.NoError(t, err)
	opened, err := os.Open(dst)
	require.NoError(t, err)
	defer opened.Close()
	require.NoError(t, CopyFile(src, dst, Overwrite(), WithFileMode(0644)))
	after, err := os.Stat(dst)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "overwrite replaced the existing inode")
	require.Equal(t, before.Mode(), after.Mode(), "creation mode must not chmod existing files")
	data, err := io.ReadAll(opened)
	require.NoError(t, err)
	require.Equal(t, "new content", string(data))
}

// TestCopyFileCompatibilityHardlinks preserves updates visible through another hardlink of the destination.
func TestCopyFileCompatibilityHardlinks(t *testing.T) {
	dir := t.TempDir()
	src, dst, other := filepath.Join(dir, "src"), filepath.Join(dir, "dst"), filepath.Join(dir, "other")
	require.NoError(t, os.WriteFile(src, []byte("replacement"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("original"), 0600))
	if err := os.Link(dst, other); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	require.NoError(t, CopyFile(src, dst, Overwrite()))
	data, err := os.ReadFile(other)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
}

// TestCopyFileCompatibilitySameFileRejects avoids truncating source data through self-copy or its hardlink.
func TestCopyFileCompatibilitySameFileRejects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0600))
	require.Error(t, CopyFile(path, path, Overwrite()))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

// TestCopyFileAtomicCompatibilityChoice verifies explicit replacement is isolated from old hardlinks.
func TestCopyFileAtomicCompatibilityChoice(t *testing.T) {
	dir := t.TempDir()
	src, dst, link := filepath.Join(dir, "src"), filepath.Join(dir, "dst"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0600))
	if err := os.Link(dst, link); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	before, err := os.Stat(dst)
	require.NoError(t, err)
	require.NoError(t, CopyFileAtomic(src, dst, Overwrite(), WithFileMode(0644)))
	after, err := os.Stat(dst)
	require.NoError(t, err)
	require.False(t, os.SameFile(before, after))
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "old", string(data))
}
