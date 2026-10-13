package memory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// TestSummaryCompatibilityChildLimit preserves the root summary in addition to the requested child limit.
func TestSummaryCompatibilityChildLimit(t *testing.T) {
	storage := &summaryProbe{memoryStorageMock: newMemoryStorageMock(), entries: []files.FileInfo{
		{Path: "/memory/alice/child", Type: files.FileTypeDirectory},
	}}
	engine, err := NewEngine(storage, Config{})
	require.NoError(t, err)
	out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", 1, 1)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "/memory/alice", out[0].Path)
	require.Equal(t, "/memory/alice/child", out[1].Path)
	require.Equal(t, 1, storage.limit, "root must not reduce or expand backend entry limit")
	require.Len(t, storage.calls, 1+3*len(out))
}
