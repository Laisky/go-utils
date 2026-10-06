package compress

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// security46Archive creates bounded members, including genuine directory entries.
func security46Archive(t *testing.T, names ...string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "fixture.zip")
	file, err := os.Create(name)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		if name[len(name)-1] == '/' {
			header.SetMode(os.ModeDir | 0755)
		} else {
			header.SetMode(0600)
		}
		entry, err := writer.CreateHeader(header)
		require.NoError(t, err)
		if !header.Mode().IsDir() {
			_, err = entry.Write([]byte("TRAP"))
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
	return name
}

// TestSecurity46UnzipParentLink confines writes even when a valid member traverses
// a pre-existing symlink. Trailing-slash directory members must also fail closed.
func TestSecurity46UnzipParentLink(t *testing.T) {
	for _, member := range []string{"redirect/target", "redirect/subdir/", "redirect/"} {
		t.Run(member, func(t *testing.T) {
			parent := t.TempDir()
			dest, outside := filepath.Join(parent, "dest"), filepath.Join(parent, "outside")
			require.NoError(t, os.Mkdir(dest, 0700))
			require.NoError(t, os.Mkdir(outside, 0700))
			sentinel := filepath.Join(outside, "target")
			require.NoError(t, os.WriteFile(sentinel, []byte("SAFE"), 0600))
			require.NoError(t, os.Symlink(outside, filepath.Join(dest, "redirect")))
			_, unzipErr := Unzip(security46Archive(t, member), dest)
			data, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "SAFE", string(data), "outside target must remain unchanged")
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			require.Len(t, entries, 1, "no outside directory or temporary file may be created")
			require.Error(t, unzipErr)
		})
	}
}

// TestSecurity46UnzipLeafLinks replaces a leaf directory entry, not its symlink
// or hardlink target. Both outside targets remain unmodified.
func TestSecurity46UnzipLeafLinks(t *testing.T) {
	for _, kind := range []string{"symbolic", "hard"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "dest")
			require.NoError(t, os.Mkdir(dest, 0700))
			sentinel := filepath.Join(parent, "sentinel")
			require.NoError(t, os.WriteFile(sentinel, []byte("SAFE"), 0600))
			leaf := filepath.Join(dest, "target")
			if kind == "symbolic" {
				require.NoError(t, os.Symlink(sentinel, leaf))
			} else {
				require.NoError(t, os.Link(sentinel, leaf))
			}
			_, err := Unzip(security46Archive(t, "target"), dest)
			require.NoError(t, err)
			data, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "SAFE", string(data))
			data, err = os.ReadFile(leaf)
			require.NoError(t, err)
			require.Equal(t, "TRAP", string(data))
			info, err := os.Lstat(leaf)
			require.NoError(t, err)
			require.True(t, info.Mode().IsRegular())
		})
	}
}

// TestSecurity46UnzipContainedLinks preserves contained relative symlinks and
// legitimate names with two dots; no substring-based traversal filter is used.
func TestSecurity46UnzipContainedLinks(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dest, "actual"), 0700))
	require.NoError(t, os.Symlink("actual", filepath.Join(dest, "alias")))
	_, err := Unzip(security46Archive(t, "alias/", "alias/a..b", "ordinary/nested/", "ordinary/nested/file"), dest)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dest, "actual", "a..b"))
	require.NoError(t, err)
	require.Equal(t, "TRAP", string(data))
}
