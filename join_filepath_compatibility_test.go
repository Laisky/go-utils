package utils

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity62RelativeReplacements preserves convenience calls using a relative
// filename, with cwd changes isolated from the concurrent parent test process.
func TestSecurity62RelativeReplacements(t *testing.T) {
	if os.Getenv("GO_UTILS_SECURITY62_REPLACE_CHILD") == "1" {
		require.NoError(t, os.WriteFile("target", []byte("old"), 0600))
		require.NoError(t, ReplaceFile("target", []byte("first"), 0600))
		got, err := os.ReadFile("target")
		require.NoError(t, err)
		require.Equal(t, "first", string(got))
		require.NoError(t, ReplaceFileAtomic("target", io.NopCloser(bytes.NewReader([]byte("second"))), 0600))
		got, err = os.ReadFile("target")
		require.NoError(t, err)
		require.Equal(t, "second", string(got))
		entries, err := os.ReadDir(".")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestSecurity62RelativeReplacements$", "-test.timeout=8s")
	child.Dir = t.TempDir()
	child.Env = append(os.Environ(), "GO_UTILS_SECURITY62_REPLACE_CHILD=1")
	output, err := child.CombinedOutput()
	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(output))
}

// TestSecurity62NativeContainment preserves internal parent traversal and rejects
// native absolute/volume paths before returning a usable output path.
func TestSecurity62NativeContainment(t *testing.T) {
	base := t.TempDir()
	root := filepath.VolumeName(base) + string(filepath.Separator)
	for _, args := range [][]string{{base, "a", "../b"}, {base, "", ""}, {root, "child"}} {
		result, err := JoinFilepath(args...)
		require.NoError(t, err)
		relative, err := filepath.Rel(filepath.Clean(args[0]), result)
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(relative))
	}
	invalid := []string{"../" + filepath.Base(base) + "-sibling/file", root + "outside"}
	if runtime.GOOS == "windows" {
		invalid = append(invalid, `C:\outside`, `C:outside`, `\outside`, `\\host\share\file`, `NUL`, `a\..\..\outside`)
	}
	for _, child := range invalid {
		result, err := JoinFilepath(base, child)
		require.Error(t, err, child)
		require.Empty(t, result)
	}
}

// TestSecurity62NativeDeviceExtensions follows the current OS's device-name
// rules. Newer Windows permits CON.txt, while older Windows can reserve it;
// a lexical native-path API must not claim every extension is a device escape.
func TestSecurity62NativeDeviceExtensions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific device name semantics")
	}
	for _, name := range []string{"CON", "NUL", "COM1", "CON.txt", "NUL.data", "COM1.log"} {
		result, err := JoinFilepath(t.TempDir(), name)
		if filepath.IsLocal(name) {
			require.NoError(t, err, name)
			require.NotEmpty(t, result)
		} else {
			require.Error(t, err, name)
			require.Empty(t, result)
		}
	}
}

// replacementFailureReader forces a finite write failure before publication.
type replacementFailureReader struct{}

// Read reports a synthetic error without writing sensitive or unbounded data.
func (replacementFailureReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestSecurity62FailedReplacementKeepsDestination verifies failure cleanup and
// preserves the previous contents when a stream cannot be read completely.
func TestSecurity62FailedReplacementKeepsDestination(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(dst, []byte("original"), 0600))
	err := ReplaceFileAtomic(dst, io.NopCloser(replacementFailureReader{}), 0600)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "original", string(got))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
