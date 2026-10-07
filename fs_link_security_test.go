package utils

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// linkFixture describes a disposable work directory with an outside victim file.
type linkFixture struct {
	work   string
	victim string
	src    string
}

// newLinkFixture creates a work directory, an outside victim containing KEEP and an
// eight-byte source file. It takes the test handle and returns the fixture paths.
func newLinkFixture(t *testing.T) linkFixture {
	t.Helper()
	parent := t.TempDir()
	fixture := linkFixture{
		work:   filepath.Join(parent, "work"),
		victim: filepath.Join(parent, "victim.txt"),
		src:    filepath.Join(parent, "src.bin"),
	}
	require.NoError(t, os.Mkdir(fixture.work, 0o700))
	require.NoError(t, os.WriteFile(fixture.victim, []byte("KEEP"), 0o600))
	require.NoError(t, os.WriteFile(fixture.src, []byte("12345678"), 0o600))
	return fixture
}

// symlinkOrSkip creates a symbolic link or skips the test when the platform refuses it.
// It takes the test handle, the link target and the link path.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// requireFileContent asserts that path is readable and holds exactly want.
// It takes the test handle, the file path and the expected content.
func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

// requireSymlink asserts that path is still a symbolic link entry.
// It takes the test handle and the link path.
func requireSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.Equal(t, os.ModeSymlink, info.Mode().Type(), "link entry must be preserved")
}

// TestIsDirWritableLinkedProbe verifies that a planted .touch link is neither
// followed nor removed; regression for issue #46.
func TestIsDirWritableLinkedProbe(t *testing.T) {
	fixture := newLinkFixture(t)
	probe := filepath.Join(fixture.work, ".touch")
	symlinkOrSkip(t, fixture.victim, probe)

	require.NoError(t, IsDirWritable(fixture.work))
	requireFileContent(t, fixture.victim, "KEEP")
	requireSymlink(t, probe)
	entries, err := os.ReadDir(fixture.work)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the pre-existing entry may remain")
}

// TestIsDirWritableKeepsExistingTouch verifies that an ordinary pre-existing
// .touch file survives the probe; regression for issue #46.
func TestIsDirWritableKeepsExistingTouch(t *testing.T) {
	fixture := newLinkFixture(t)
	touch := filepath.Join(fixture.work, ".touch")
	require.NoError(t, os.WriteFile(touch, []byte("mine"), 0o600))

	require.NoError(t, IsDirWritable(fixture.work))
	requireFileContent(t, touch, "mine")
	entries, err := os.ReadDir(fixture.work)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the probe must remove only its own file")
}

// TestIsDirWritableMissingDirectory verifies that an absent directory is reported
// as an error; regression for issue #46.
func TestIsDirWritableMissingDirectory(t *testing.T) {
	require.Error(t, IsDirWritable(filepath.Join(t.TempDir(), "missing")))
}

// TestCopyFileDanglingLink verifies that the default no-overwrite copy refuses a
// dangling destination link and never creates its target; regression for issue #46.
func TestCopyFileDanglingLink(t *testing.T) {
	fixture := newLinkFixture(t)
	outside := filepath.Join(filepath.Dir(fixture.work), "outside.txt")
	dst := filepath.Join(fixture.work, "dst")
	symlinkOrSkip(t, outside, dst)

	require.Error(t, CopyFile(fixture.src, dst))
	_, err := os.Lstat(outside)
	require.ErrorIs(t, err, os.ErrNotExist, "copy must not create a file through a dangling link")
	requireSymlink(t, dst)
}

// TestCopyFileExistingLinkNoOverwrite verifies that a link to an existing file is
// treated as a destination collision; regression for issue #46.
func TestCopyFileExistingLinkNoOverwrite(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	symlinkOrSkip(t, fixture.victim, dst)

	require.Error(t, CopyFile(fixture.src, dst))
	requireFileContent(t, fixture.victim, "KEEP")
	requireSymlink(t, dst)
}

// TestCopyFileOverwriteLinks verifies that explicit overwrite permission does not
// authorize writing through final or dangling links; regression for issue #46.
func TestCopyFileOverwriteLinks(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "dangling"}[dangling], func(t *testing.T) {
			fixture := newLinkFixture(t)
			target := fixture.victim
			if dangling {
				target = filepath.Join(filepath.Dir(fixture.work), "outside.txt")
			}
			dst := filepath.Join(fixture.work, "dst")
			symlinkOrSkip(t, target, dst)

			err := CopyFile(fixture.src, dst, Overwrite())
			require.Error(t, err)
			require.Contains(t, err.Error(), "not a regular file")
			requireFileContent(t, fixture.victim, "KEEP")
			requireSymlink(t, dst)
			if dangling {
				_, err = os.Lstat(target)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

// TestCopyFileOverwriteDirectory verifies that a directory destination is rejected
// and left intact; regression for issue #46.
func TestCopyFileOverwriteDirectory(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	require.NoError(t, os.Mkdir(dst, 0o700))

	require.Error(t, CopyFile(fixture.src, dst, Overwrite()))
	info, err := os.Lstat(dst)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// TestCopyFileOverwriteHardLink verifies that replacing a hard-linked destination
// swaps the directory entry instead of rewriting the shared inode; regression for issue #46.
func TestCopyFileOverwriteHardLink(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	if err := os.Link(fixture.victim, dst); err != nil {
		t.Skipf("hard links unsupported: %v", err)
	}

	require.NoError(t, CopyFile(fixture.src, dst, Overwrite()))
	requireFileContent(t, dst, "12345678")
	requireFileContent(t, fixture.victim, "KEEP")
}

// TestCopyFileOverwriteRegular verifies the legitimate replacement path, including
// the requested mode for the replacement file; regression for issue #46.
func TestCopyFileOverwriteRegular(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	require.NoError(t, os.WriteFile(dst, []byte("previous content"), 0o600))

	require.NoError(t, CopyFile(fixture.src, dst, Overwrite(), WithFileMode(0o600)))
	requireFileContent(t, dst, "12345678")
	entries, err := os.ReadDir(fixture.work)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file may remain after publication")
}

// TestCopyFileNewDestination verifies the ordinary exclusive creation path and
// that a missing source leaves no destination behind; regression for issue #46.
func TestCopyFileNewDestination(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "nested", "dst")
	require.NoError(t, CopyFile(fixture.src, dst))
	requireFileContent(t, dst, "12345678")

	missing := filepath.Join(fixture.work, "missing-dst")
	require.Error(t, CopyFile(filepath.Join(fixture.work, "no-such-source"), missing))
	_, err := os.Lstat(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestMoveFileDanglingLink verifies that MoveFile keeps its source when the
// destination is a dangling link; regression for issue #46.
func TestMoveFileDanglingLink(t *testing.T) {
	fixture := newLinkFixture(t)
	outside := filepath.Join(filepath.Dir(fixture.work), "outside.txt")
	dst := filepath.Join(fixture.work, "dst")
	symlinkOrSkip(t, outside, dst)

	require.Error(t, MoveFile(fixture.src, dst))
	requireFileContent(t, fixture.src, "12345678")
	_, err := os.Lstat(outside)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestCopyFileOverwriteRace flips the destination between a regular file and a
// link to the victim while overwrite copies run. The victim must never change;
// regression for issue #46.
func TestCopyFileOverwriteRace(t *testing.T) {
	fixture := newLinkFixture(t)
	dst := filepath.Join(fixture.work, "dst")
	require.NoError(t, os.WriteFile(dst, []byte("seed"), 0o600))
	probe := filepath.Join(fixture.work, "probe-link")
	if err := os.Symlink(fixture.victim, probe); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	require.NoError(t, os.Remove(probe))

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load() && i < 5000; i++ {
			staging := filepath.Join(fixture.work, "staging-link")
			if err := os.Symlink(fixture.victim, staging); err != nil {
				continue
			}
			if err := os.Rename(staging, dst); err != nil {
				_ = os.Remove(staging)
			}
		}
	}()
	for range 200 {
		err := CopyFile(fixture.src, dst, Overwrite())
		if err != nil {
			require.True(t, strings.Contains(err.Error(), "not a regular file") ||
				strings.Contains(err.Error(), "replace"), err.Error())
		}
	}
	stop.Store(true)
	wg.Wait()
	requireFileContent(t, fixture.victim, "KEEP")
}
