package local

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestReadRangeAllocation ensures a one-byte request does not allocate an
// entire four-megabyte sparse file. The generous threshold excludes small
// runtime and filesystem overhead rather than relying on an exact allocation count.
func TestReadRangeAllocation(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(Config{RootDir: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.Write(t.Context(), "demo", "/large", "x", storage.WriteModeTruncate, 0))
	require.NoError(t, os.Truncate(filepath.Join(root, "demo", "large"), 4*1024*1024))
	_, err = engine.Read(t.Context(), "demo", "/large", 0, 0)
	require.NoError(t, err)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := engine.Read(t.Context(), "demo", "/large", 0, 1)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.Equal(t, "x", got)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(512*1024), "one-byte ranged read allocated %d bytes", after.TotalAlloc-before.TotalAlloc)
}

// TestSearchDefaultFileBudget uses only small empty files to prove that a
// result limit of one does not permit scanning arbitrarily many nonmatches.
func TestSearchDefaultFileBudget(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(Config{RootDir: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	for i := 0; i < 1025; i++ {
		require.NoError(t, engine.Write(t.Context(), "demo", fmt.Sprintf("/items/%04d", i), "", storage.WriteModeTruncate, 0))
	}
	require.NoError(t, engine.Write(t.Context(), "demo", "/items/last", "needle", storage.WriteModeTruncate, 0))
	got, err := engine.Search(t.Context(), "demo", "needle", "/items", 1)
	require.Error(t, err, "search scanned more than 1024 files before returning %d matches", len(got))
	require.Empty(t, got)
}

// TestSearchDefaultResultByteBudget bounds retained matching content separately
// from the number of matches, using three independently permitted small files.
func TestSearchDefaultResultByteBudget(t *testing.T) {
	engine, err := NewEngine(Config{RootDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	body := "needle" + strings.Repeat("a", 3*1024*1024)
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, engine.Write(t.Context(), "demo", "/items/"+name, body, storage.WriteModeTruncate, 0))
	}
	got, err := engine.Search(t.Context(), "demo", "needle", "/items", 3)
	require.Error(t, err, "returned %d whole matching files with no aggregate byte bound", len(got))
	require.Empty(t, got)
}
