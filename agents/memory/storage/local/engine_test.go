package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestEngineListAndSearchSkipSymlink verifies that traversal and search ignore symlink entries.
func TestEngineListAndSearchSkipSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	require.NoError(t, engine.Write(ctx, project, "/docs/readme.txt", "regular file", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, engine.Write(ctx, project, "/other/target.txt", "secret-symlink-token", memorystorage.WriteModeTruncate, 0))

	projectRoot := filepath.Join(rootDir, project)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "docs"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "other", "target.txt"), filepath.Join(projectRoot, "docs", "target-link.txt")))

	entries, _, err := engine.List(ctx, project, "/docs", 8, 100)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, "/docs/target-link.txt", entry.Path)
	}

	chunks, err := engine.Search(ctx, project, "secret-symlink-token", "/docs", 5)
	require.NoError(t, err)
	require.Empty(t, chunks)
}

// TestEngineListSkipSymlinkLoop verifies that listing does not recurse through symlink loops.
func TestEngineListSkipSymlinkLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	require.NoError(t, engine.Write(ctx, project, "/loop/a.txt", "loop file", memorystorage.WriteModeTruncate, 0))

	projectRoot := filepath.Join(rootDir, project)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "loop"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "loop"), filepath.Join(projectRoot, "loop", "self")))

	entries, _, err := engine.List(ctx, project, "/loop", 8, 100)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, "/loop/self", entry.Path)
	}
}

// TestEngineSearchSkipsLargeFiles verifies that search ignores oversized files to bound memory and latency.
func TestEngineSearchSkipsLargeFiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	largeBody := "search-token-large " + strings.Repeat("a", maxSearchFileBytes)
	require.NoError(t, engine.Write(ctx, project, "/docs/large.txt", largeBody, memorystorage.WriteModeTruncate, 0))

	chunks, err := engine.Search(ctx, project, "search-token-large", "/docs", 5)
	require.NoError(t, err)
	require.Empty(t, chunks)
}

// TestEnginePermissionErrorsAreSurfaced verifies that list and search return actionable permission errors.
func TestEnginePermissionErrorsAreSurfaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission model differs on windows")
	}
	if isRunningAsRoot() {
		t.Skip("permission-denied expectations are unreliable when running as root")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir := t.TempDir()
	project := "memory-storage-local"
	engine, err := NewEngine(Config{RootDir: rootDir})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, engine.Close())
	}()

	require.NoError(t, engine.Write(ctx, project, "/restricted/secret.txt", "forbidden", memorystorage.WriteModeTruncate, 0))

	restrictedDir := filepath.Join(rootDir, project, "restricted")
	require.NoError(t, os.Chmod(restrictedDir, 0))
	t.Cleanup(func() {
		_ = os.Chmod(restrictedDir, 0o755)
	})

	_, _, listErr := engine.List(ctx, project, "/", 8, 100)
	require.Error(t, listErr)
	require.Contains(t, strings.ToLower(listErr.Error()), "permission denied")

	_, searchErr := engine.Search(ctx, project, "forbidden", "/", 5)
	require.Error(t, searchErr)
	require.Contains(t, strings.ToLower(searchErr.Error()), "permission denied")
}
