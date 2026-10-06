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
		invalid = append(invalid, `C:\outside`, `C:outside`, `\outside`, `\\host\share\file`, `NUL`, `CON.txt`, `a\..\..\outside`)
	}
	for _, child := range invalid {
		result, err := JoinFilepath(base, child)
		require.Error(t, err, child)
		require.Empty(t, result)
	}
}
