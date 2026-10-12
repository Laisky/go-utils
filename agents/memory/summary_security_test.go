package memory

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// summaryProbe records storage effects and can inject hostile listing results.
type summaryProbe struct {
	*memoryStorageMock
	calls        []string
	depth, limit int
	entries      []files.FileInfo
}

// List records the requested bounds and returns injected entries or real mock data.
func (s *summaryProbe) List(ctx context.Context, project, p string, depth, limit int) ([]files.FileInfo, bool, error) {
	s.calls = append(s.calls, "list:"+p)
	s.depth, s.limit = depth, limit
	if s.entries != nil {
		return s.entries, false, nil
	}
	return s.memoryStorageMock.List(ctx, project, p, depth, limit)
}

// Stat records metadata access before delegating to in-memory storage.
func (s *summaryProbe) Stat(ctx context.Context, project, p string) (files.FileInfo, error) {
	s.calls = append(s.calls, "stat:"+p)
	return s.memoryStorageMock.Stat(ctx, project, p)
}

// Read records data access before delegating to in-memory storage.
func (s *summaryProbe) Read(ctx context.Context, project, p string, offset, length int64) (string, error) {
	s.calls = append(s.calls, "read:"+p)
	return s.memoryStorageMock.Read(ctx, project, p, offset, length)
}

// Write records mutations before delegating to in-memory storage.
func (s *summaryProbe) Write(ctx context.Context, project, p, body string, mode files.WriteMode, offset int64) error {
	s.calls = append(s.calls, "write:"+p)
	return s.memoryStorageMock.Write(ctx, project, p, body, mode, offset)
}

// TestSummarySessionBoundary checks rejected roots produce no storage effects,
// including no cross-session .abstract creation through this listing operation.
func TestSummarySessionBoundary(t *testing.T) {
	for _, root := range []string{"/memory/bob", "/memory/alice-other", "/", "/memory/alice/../bob", "memory/alice", "/memory/alice/./x"} {
		t.Run(root, func(t *testing.T) {
			storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock()}
			engine, err := NewEngine(storage, Config{})
			require.NoError(t, err)
			summaries, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", root, 2, 5)
			require.Error(t, err)
			require.Empty(t, summaries)
			require.Empty(t, storage.calls)
			require.Empty(t, storage.files)
		})
	}
}

// TestSummaryBackendBoundary rejects backend paths outside the requested root
// before issuing any metadata calls or writes, even for sibling-prefix matches.
func TestSummaryBackendBoundary(t *testing.T) {
	for _, p := range []string{"/memory/bob", "/memory/alice-other/x", "/memory/alice/sub/../../bob"} {
		t.Run(p, func(t *testing.T) {
			storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock(), entries: []files.FileInfo{{Path: p, Type: files.FileTypeDirectory}}}
			engine, err := NewEngine(storage, Config{})
			require.NoError(t, err)
			out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", 2, 5)
			require.Error(t, err)
			require.Empty(t, out)
			require.Equal(t, []string{"list:/memory/alice"}, storage.calls)
		})
	}
}

// TestSummaryRequestBounds checks caller-controlled positive limits are clamped
// and large abstract bodies are rejected without an unbounded storage Read.
func TestSummaryRequestBounds(t *testing.T) {
	t.Run("limits", func(t *testing.T) {
		storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock()}
		engine, err := NewEngine(storage, Config{})
		require.NoError(t, err)
		_, err = engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", math.MaxInt, math.MaxInt)
		require.NoError(t, err)
		require.LessOrEqual(t, storage.depth, 32)
		require.LessOrEqual(t, storage.limit, 200)
	})
	t.Run("large_abstract", func(t *testing.T) {
		storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock()}
		storage.files["demo:/memory/alice/.abstract"] = strings.Repeat("a", 128*1024)
		engine, err := NewEngine(storage, Config{})
		require.NoError(t, err)
		out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", 2, 5)
		require.Error(t, err)
		require.Empty(t, out)
		require.NotContains(t, storage.calls, "read:/memory/alice/.abstract")
	})
	t.Run("backend_exceeds_limit", func(t *testing.T) {
		storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock(), entries: []files.FileInfo{
			{Path: "/memory/alice/a", Type: files.FileTypeDirectory},
			{Path: "/memory/alice/b", Type: files.FileTypeDirectory},
		}}
		engine, err := NewEngine(storage, Config{})
		require.NoError(t, err)
		out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", 2, 1)
		require.Error(t, err)
		require.Empty(t, out)
		require.Equal(t, []string{"list:/memory/alice"}, storage.calls)
	})
}
