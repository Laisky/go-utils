package fileguard

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegularIdentity rejects a swapped inode before returning a readable handle.
func TestRegularIdentity(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("original"), 0600))
	root, err := OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	info, err := root.Lstat("file")
	require.NoError(t, err)
	f, err := OpenRegular(root, "file", info)
	require.NoError(t, err)
	data, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	require.NoError(t, f.Close())
	require.NoError(t, os.Rename(file, filepath.Join(dir, "original")))
	require.NoError(t, os.WriteFile(file, []byte("replacement"), 0600))
	f, err = OpenRegular(root, "file", info)
	require.Error(t, err)
	require.Nil(t, f)
}

// TestDirectoryIdentity rejects directory replacement and unclean relative paths.
func TestDirectoryIdentity(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "child"), 0700))
	root, err := OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	info, err := root.Lstat("child")
	require.NoError(t, err)
	require.NoError(t, os.Rename(filepath.Join(dir, "child"), filepath.Join(dir, "saved")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "child"), 0700))
	child, err := OpenDirectory(root, "child", info)
	require.Error(t, err)
	require.Nil(t, child)
	for _, name := range []string{"", "../child", "child/", "child/../child"} {
		child, err = OpenDirectory(root, name, info)
		require.Error(t, err)
		require.Nil(t, child)
	}
}

// TestRootedSymlinkReplacement checks both file and directory races against outside links.
func TestRootedSymlinkReplacement(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "root")
			require.NoError(t, os.Mkdir(dir, 0700))
			target := filepath.Join(dir, "entry")
			outside := filepath.Join(parent, "outside")
			if directory {
				require.NoError(t, os.Mkdir(target, 0700))
				require.NoError(t, os.Mkdir(outside, 0700))
			} else {
				require.NoError(t, os.WriteFile(target, []byte("original"), 0600))
				require.NoError(t, os.WriteFile(outside, []byte("outside"), 0600))
			}
			root, err := OpenRoot(dir)
			require.NoError(t, err)
			defer root.Close()
			info, err := root.Lstat("entry")
			require.NoError(t, err)
			require.NoError(t, os.Rename(target, filepath.Join(dir, "saved")))
			if err := os.Symlink(outside, target); err != nil {
				t.Skipf("symlink unsupported: %v", err)
			}
			if directory {
				f, err := OpenDirectory(root, "entry", info)
				require.Error(t, err)
				require.Nil(t, f)
			} else {
				f, err := OpenRegular(root, "entry", info)
				require.Error(t, err)
				require.Nil(t, f)
			}
		})
	}
}
