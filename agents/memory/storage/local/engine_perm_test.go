package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestNewEngineSecureDefaultPermissions verifies the local storage engine creates
// its root directory and written files with owner-only permissions by default, so
// sensitive memory data is not group/world-readable on shared hosts.
func TestNewEngineSecureDefaultPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}

	rootDir := filepath.Join(t.TempDir(), "memory-root")
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	rootInfo, err := os.Stat(rootDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), rootInfo.Mode().Perm(), "root dir must be owner-only")

	ctx := context.Background()
	require.NoError(t, engine.Write(
		ctx,
		"proj",
		"/memory/session/state.json",
		"{}",
		memorystorage.WriteModeTruncate,
		0,
	))

	// The per-session directory created via MkdirAll must be owner-only.
	dirInfo, err := os.Stat(filepath.Join(rootDir, "proj", "memory", "session"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm(), "session dir must be owner-only")

	// The written file must be owner read/write only.
	fileInfo, err := os.Stat(filepath.Join(rootDir, "proj", "memory", "session", "state.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(), "written file must be owner-only")
}
