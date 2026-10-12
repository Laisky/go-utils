package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// TestReadRangeBudgetBoundaries preserves range/EOF semantics while refusing
// unbounded full reads and invalid negative lengths before allocating data.
func TestReadRangeBudgetBoundaries(t *testing.T) {
	engine, err := NewEngine(Config{RootDir: t.TempDir(), Limits: Limits{MaxReadBytes: 4}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.Write(t.Context(), "demo", "/x", "0123456789", storage.WriteModeTruncate, 0))
	for _, tc := range []struct {
		offset, length int64
		want           string
	}{{2, 3, "234"}, {8, -1, "89"}, {10, -1, ""}, {10, 100, ""}, {0, 0, ""}} {
		got, err := engine.Read(t.Context(), "demo", "/x", tc.offset, tc.length)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	_, err = engine.Read(t.Context(), "demo", "/x", 0, -1)
	require.ErrorIs(t, err, ErrBudgetExceeded)
	_, err = engine.Read(t.Context(), "demo", "/x", 0, -2)
	require.Error(t, err)
	_, err = engine.Read(t.Context(), "demo", "/x", 11, 1)
	require.Error(t, err)
	got, err := engine.Read(t.Context(), "demo", "/absent", 0, -1)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestSearchIndependentBudgets checks each counter using tiny fixtures rather
// than relying on expensive resource exhaustion or elapsed-time measurements.
func TestSearchIndependentBudgets(t *testing.T) {
	tests := []struct {
		name   string
		limits Limits
	}{
		{"files", Limits{MaxSearchFiles: 1}},
		{"entries", Limits{MaxSearchEntries: 1}},
		{"scanned", Limits{MaxSearchBytes: 5}},
		{"retained", Limits{MaxSearchResultBytes: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewEngine(Config{RootDir: t.TempDir(), Limits: tc.limits})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, e.Close()) })
			for _, name := range []string{"a", "b"} {
				require.NoError(t, e.Write(t.Context(), "demo", "/items/"+name, "needle", storage.WriteModeTruncate, 0))
			}
			out, err := e.Search(t.Context(), "demo", "needle", "/items", 2)
			require.ErrorIs(t, err, ErrBudgetExceeded)
			require.Empty(t, out)
		})
	}
}

// TestLocalReadSearchCancellation checks expired operation deadlines and caller
// cancellation without relying on a large or slow filesystem workload.
func TestLocalReadSearchCancellation(t *testing.T) {
	e, err := NewEngine(Config{RootDir: t.TempDir(), Limits: Limits{SearchTimeout: time.Nanosecond}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Close()) })
	require.NoError(t, e.Write(t.Context(), "demo", "/x", "needle", storage.WriteModeTruncate, 0))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = e.Read(ctx, "demo", "/x", 0, 1)
	require.ErrorIs(t, err, context.Canceled)
	_, err = e.Search(ctx, "demo", "needle", "/", 1)
	require.ErrorIs(t, err, context.Canceled)
	out, err := e.Search(t.Context(), "demo", "needle", "/", 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, out)
}

// TestSearchMeasuredByteBudget inspects actual counters after a bounded scan
// fails, proving nonmatching files cannot exceed a small aggregate byte limit.
func TestSearchMeasuredByteBudget(t *testing.T) {
	e, err := NewEngine(Config{RootDir: t.TempDir(), Limits: Limits{MaxSearchBytes: 5}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Close()) })
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, e.Write(t.Context(), "demo", "/"+name, "abc", storage.WriteModeTruncate, 0))
	}
	root, err := e.openProjectRoot("demo", false)
	require.NoError(t, err)
	defer func() { require.NoError(t, root.Close()) }()
	state := &searchState{ctx: t.Context(), root: root, limits: e.limits, query: "none", queryLower: "none", resultLimit: 1, entries: 1}
	require.ErrorIs(t, state.walk(".", 0), ErrBudgetExceeded)
	require.Equal(t, int64(3), state.scanned)
	require.Equal(t, 2, state.files)
	require.Empty(t, state.chunks)
}
