//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestTransactionReadOnlyDoesNotStage requires zero generation creation or publication for existing snapshots.
func TestTransactionReadOnlyDoesNotStage(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/value", "value", storage.WriteModeTruncate, 0))
	pointer, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	before, err := filepath.Glob(filepath.Join(engine.root, "generation-*"))
	require.NoError(t, err)
	require.NoError(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		during, globErr := filepath.Glob(filepath.Join(engine.root, "generation-*"))
		require.NoError(t, globErr)
		require.Equal(t, before, during, "a read-only transaction must not stage a full snapshot")
		body, readErr := view.Read(ctx, "p", "/value", 0, -1)
		require.NoError(t, readErr)
		require.Equal(t, "value", body)
		afterRead, globErr := filepath.Glob(filepath.Join(engine.root, "generation-*"))
		require.NoError(t, globErr)
		require.Equal(t, before, afterRead, "actual reads must not stage a snapshot")
		return nil
	}))
	after, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	require.Equal(t, pointer, after)
	// Positive control: a real write still publishes a new isolated generation.
	require.NoError(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		return view.Write(ctx, "p", "/value", "changed", storage.WriteModeTruncate, 0)
	}))
	after, err = os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	require.NotEqual(t, pointer, after)
}

// TestTransactionPreservesUntouchedTimestamps prevents unrelated writes from refreshing file or directory ages.
func TestTransactionPreservesUntouchedTimestamps(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/session-a/sub/value", "old", storage.WriteModeTruncate, 0))
	pointer, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	past := time.Date(2020, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	relatives := []string{"p/session-a/sub/value", "p/session-a/sub", "p/session-a"}
	for _, relative := range relatives {
		require.NoError(t, os.Chtimes(filepath.Join(engine.root, string(pointer), relative), past, past))
	}
	before, err := engine.Stat(ctx, "p", "/session-a/sub/value")
	require.NoError(t, err)
	require.NoError(t, engine.Write(ctx, "p", "/session-b/value", "new", storage.WriteModeTruncate, 0))
	after, err := engine.Stat(ctx, "p", "/session-a/sub/value")
	require.NoError(t, err)
	require.Equal(t, before.UpdatedAt, after.UpdatedAt)
	pointer, err = os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	for _, relative := range relatives {
		info, statErr := os.Stat(filepath.Join(engine.root, string(pointer), relative))
		require.NoError(t, statErr)
		require.True(t, past.Equal(info.ModTime()), "unchanged entry %s has changed mtime: %s", relative, info.ModTime())
	}
	// Positive control: deliberately updating this file must change its age.
	require.NoError(t, engine.Write(ctx, "p", "/session-a/sub/value", "changed", storage.WriteModeTruncate, 0))
	changed, err := engine.Stat(ctx, "p", "/session-a/sub/value")
	require.NoError(t, err)
	require.NotEqual(t, before.UpdatedAt, changed.UpdatedAt)
}

// TestTransactionEmptyNoopDoesNotCreateSnapshot checks callbacks without operations have no staging side effects.
func TestTransactionEmptyNoopDoesNotCreateSnapshot(t *testing.T) {
	engine := newTestEngine(t)
	require.NoError(t, engine.Transaction(context.Background(), "p", "s", func(storage.Engine) error { return nil }))
	generations, err := filepath.Glob(filepath.Join(engine.root, "generation-*"))
	require.NoError(t, err)
	require.Empty(t, generations)
	_, err = os.Stat(filepath.Join(engine.root, "current"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestTransactionPreservesUntouchedPermissions checks copying does not reapply the process umask.
func TestTransactionPreservesUntouchedPermissions(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/untouched/value", "kept", storage.WriteModeTruncate, 0))
	pointer, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(engine.root, string(pointer), "p/untouched/value"), 0o646))
	require.NoError(t, os.Chmod(filepath.Join(engine.root, string(pointer), "p/untouched"), 0o752))
	require.NoError(t, engine.Write(ctx, "p", "/other/value", "new", storage.WriteModeTruncate, 0))
	pointer, err = os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	file, err := os.Stat(filepath.Join(engine.root, string(pointer), "p/untouched/value"))
	require.NoError(t, err)
	directory, err := os.Stat(filepath.Join(engine.root, string(pointer), "p/untouched"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o646), file.Mode().Perm())
	require.Equal(t, os.FileMode(0o752), directory.Mode().Perm())
}
