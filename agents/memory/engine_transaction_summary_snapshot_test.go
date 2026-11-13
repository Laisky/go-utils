//go:build linux

package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/transactionlocal"
)

// TestTransactionalSummarySnapshotMetadata covers the public directory summary
// path: an existing abstract causes no publication, and another session's turn
// preserves its filesystem-derived timestamp despite a full write snapshot.
func TestTransactionalSummarySnapshotMetadata(t *testing.T) {
	root := privateTransactionRoot(t)
	backend, err := transactionlocal.NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	require.NoError(t, backend.Write(t.Context(), recoveryProject, "/memory/other/.abstract", "existing abstract", storage.WriteModeTruncate, 0))
	require.NoError(t, backend.Write(t.Context(), recoveryProject, "/bulk/history", strings.Repeat("x", 1024*1024), storage.WriteModeTruncate, 0))
	pointer, err := os.ReadFile(filepath.Join(root, "current"))
	require.NoError(t, err)
	past := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	// Set up an old metadata fixture before any concurrent operation begins.
	require.NoError(t, os.Chtimes(filepath.Join(root, string(pointer), recoveryProject, "memory/other/.abstract"), past, past))
	engine, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	before, err := engine.ListDirWithAbstract(t.Context(), recoveryProject, "other", "", 1, 10)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Equal(t, past.Format(time.RFC3339Nano), before[0].UpdatedAt)
	unchanged, err := os.ReadFile(filepath.Join(root, "current"))
	require.NoError(t, err)
	require.Equal(t, pointer, unchanged, "read-only public summary call must not publish the unrelated bulk history")
	require.NoError(t, engine.AfterTurn(t.Context(), recoveryInput("one")))
	after, err := engine.ListDirWithAbstract(t.Context(), recoveryProject, "other", "", 1, 10)
	require.NoError(t, err)
	require.Equal(t, before, after, "unrelated session writes cannot refresh summary timestamps")
}
