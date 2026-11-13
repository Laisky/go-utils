//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCopyTreePopulatedReadOnlyDirectory proves copying works without write permission on the source directory.
// Its nonroot permission control must actually reject child creation, rather than
// silently treating a privileged process as evidence of ordinary user behavior.
func TestCopyTreePopulatedReadOnlyDirectory(t *testing.T) {
	require.NotZero(t, os.Geteuid(), "permission behavior must be tested in a nonroot process")
	for _, mode := range []os.FileMode{0o500, 0o550, 0o750} {
		t.Run(mode.String(), func(t *testing.T) {
			source := privateTestRoot(t)
			target := privateTestRoot(t)
			directory := filepath.Join(source, "protected")
			child := filepath.Join(directory, "nested")
			require.NoError(t, os.MkdirAll(child, 0o700))
			sourceFile := filepath.Join(child, "value")
			require.NoError(t, os.WriteFile(sourceFile, []byte("historical payload"), 0o400))
			past := time.Date(2020, time.February, 3, 4, 5, 6, 123456789, time.UTC)
			require.NoError(t, os.Chtimes(sourceFile, past, past))
			require.NoError(t, os.Chmod(child, mode))
			require.NoError(t, os.Chmod(directory, mode))
			require.NoError(t, os.Chtimes(child, past, past))
			require.NoError(t, os.Chtimes(directory, past, past))
			// Only disposable fixture directories are made writable during test cleanup.
			t.Cleanup(func() {
				require.NoError(t, os.Chmod(directory, 0o700))
				require.NoError(t, os.Chmod(child, 0o700))
				for _, relative := range []string{"protected", "protected/nested"} {
					destination := filepath.Join(target, relative)
					if _, err := os.Stat(destination); os.IsNotExist(err) {
						continue
					}
					require.NoError(t, os.Chmod(destination, 0o700))
				}
			})
			content, err := os.ReadFile(sourceFile)
			require.NoError(t, err)
			require.Equal(t, "historical payload", string(content))
			if mode&0o200 == 0 {
				err = os.WriteFile(filepath.Join(directory, "permission-control"), []byte("denied"), 0o600)
				require.ErrorIs(t, err, os.ErrPermission, "the fixture must enforce real owner write restrictions")
			}
			require.NoError(t, copyTree(context.Background(), source, target))
			copiedFile := filepath.Join(target, "protected/nested/value")
			content, err = os.ReadFile(copiedFile)
			require.NoError(t, err)
			require.Equal(t, "historical payload", string(content))
			for _, relative := range []string{"protected", "protected/nested", "protected/nested/value"} {
				original, statErr := os.Stat(filepath.Join(source, relative))
				require.NoError(t, statErr)
				copied, statErr := os.Stat(filepath.Join(target, relative))
				require.NoError(t, statErr)
				require.Equal(t, original.Mode().Perm(), copied.Mode().Perm(), relative)
				require.True(t, past.Equal(copied.ModTime()), "copied %s must retain historical mtime", relative)
				require.False(t, os.SameFile(original, copied), "snapshot files and directories must be independently copied")
			}
		})
	}
}
