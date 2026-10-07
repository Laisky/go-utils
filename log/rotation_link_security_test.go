package log

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rotationDay1 is the fixed first rotation window used by the link tests.
var rotationDay1 = time.Date(2025, time.January, 1, 10, 30, 0, 0, time.UTC)

// newFixedRotationWriter builds a writer for dir/app.log using the default
// {logger}-YYYYMMDD.log pattern and a fixed clock. It takes the test handle and
// the log directory, and returns the writer, which is closed on cleanup.
func newFixedRotationWriter(t *testing.T, dir string) *rotationWriter {
	t.Helper()
	writer, err := newRotationWriter(filepath.Join(dir, "app.log"), 0, "", "")
	require.NoError(t, err)
	writer.now = func() time.Time { return rotationDay1 }
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	return writer
}

// rotationLinkFixture creates a log directory and an outside sentinel file. It
// takes the test handle and returns the log directory and sentinel path.
func rotationLinkFixture(t *testing.T) (string, string) {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "logs")
	require.NoError(t, os.Mkdir(dir, 0o700))
	sentinel := filepath.Join(parent, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("KEEP"), 0o600))
	return dir, sentinel
}

// rotationSymlinkOrSkip creates a symbolic link or skips when the host refuses.
// It takes the test handle, the link target and the link path.
func rotationSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// requireRotationContent asserts that path holds exactly want.
// It takes the test handle, the file path and the expected content.
func requireRotationContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(data))
}

// TestRotationRejectsSymlinkAtDailyFile verifies that a link planted at the
// computed daily filename is rejected without mutating its target; regression for issue #46.
func TestRotationRejectsSymlinkAtDailyFile(t *testing.T) {
	dir, sentinel := rotationLinkFixture(t)
	rotationSymlinkOrSkip(t, sentinel, filepath.Join(dir, "app-20250101.log"))
	writer := newFixedRotationWriter(t, dir)

	_, err := writer.Write([]byte("log line\n"))
	require.Error(t, err)
	requireRotationContent(t, sentinel, "KEEP")
}

// TestRotationRejectsDanglingSymlink verifies that a dangling link at the daily
// filename never creates its outside target; regression for issue #46.
func TestRotationRejectsDanglingSymlink(t *testing.T) {
	dir, sentinel := rotationLinkFixture(t)
	outside := filepath.Join(filepath.Dir(sentinel), "outside.log")
	rotationSymlinkOrSkip(t, outside, filepath.Join(dir, "app-20250101.log"))
	writer := newFixedRotationWriter(t, dir)

	_, err := writer.Write([]byte("log line\n"))
	require.Error(t, err)
	_, err = os.Lstat(outside)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestRotationRejectsSymlinkOnRollover verifies that the same destination policy
// applies to every rollover, not only the first open; regression for issue #46.
func TestRotationRejectsSymlinkOnRollover(t *testing.T) {
	dir, sentinel := rotationLinkFixture(t)
	writer := newFixedRotationWriter(t, dir)
	_, err := writer.Write([]byte("day one\n"))
	require.NoError(t, err)

	rotationSymlinkOrSkip(t, sentinel, filepath.Join(dir, "app-20250102.log"))
	writer.now = func() time.Time { return rotationDay1.Add(24 * time.Hour) }
	_, err = writer.Write([]byte("day two\n"))
	require.Error(t, err)
	requireRotationContent(t, sentinel, "KEEP")
	requireRotationContent(t, filepath.Join(dir, "app-20250101.log"), "day one\n")
}

// TestRotationRejectsHardLink verifies that a regular file with a second link,
// which could alias an unrelated file, is rejected on Unix; regression for issue #46.
func TestRotationRejectsHardLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("link counts are not inspected on Windows")
	}
	dir, sentinel := rotationLinkFixture(t)
	if err := os.Link(sentinel, filepath.Join(dir, "app-20250101.log")); err != nil {
		t.Skipf("hard links unsupported: %v", err)
	}
	writer := newFixedRotationWriter(t, dir)

	_, err := writer.Write([]byte("log line\n"))
	require.Error(t, err)
	requireRotationContent(t, sentinel, "KEEP")
}

// TestRotationRejectsDirectoryAtDailyFile verifies that a directory planted at the
// daily filename is rejected; regression for issue #46.
func TestRotationRejectsDirectoryAtDailyFile(t *testing.T) {
	dir, _ := rotationLinkFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "app-20250101.log"), 0o700))
	writer := newFixedRotationWriter(t, dir)

	_, err := writer.Write([]byte("log line\n"))
	require.Error(t, err)
}

// TestRotationAppendsExistingRegularFile verifies that an existing private daily
// file is still reused in append mode; regression for issue #46.
func TestRotationAppendsExistingRegularFile(t *testing.T) {
	dir, _ := rotationLinkFixture(t)
	daily := filepath.Join(dir, "app-20250101.log")
	require.NoError(t, os.WriteFile(daily, []byte("earlier\n"), 0o600))
	writer := newFixedRotationWriter(t, dir)

	_, err := writer.Write([]byte("later\n"))
	require.NoError(t, err)
	requireRotationContent(t, daily, "earlier\nlater\n")
}

// TestRotationPrivateModes verifies that new log directories default to 0700 and
// new daily files keep 0600; regression for issue #46.
func TestRotationPrivateModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	writer := newFixedRotationWriter(t, dir)
	_, err := writer.Write([]byte("log line\n"))
	require.NoError(t, err)

	for _, path := range []string{dir, filepath.Dir(dir)} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0), info.Mode().Perm()&0o077, path)
	}
	info, err := os.Stat(filepath.Join(dir, "app-20250101.log"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), info.Mode().Perm()&0o077)
}
