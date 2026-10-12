package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSearchDisappearingFile verifies that a file removed after its metadata
// was inspected remains a harmless race rather than failing an entire search.
func TestSearchDisappearingFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "vanishing.txt")
	require.NoError(t, os.WriteFile(filename, []byte("fixture"), 0o600))
	info, err := os.Stat(filename)
	require.NoError(t, err)
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, os.Remove(filename))
	state := searchState{ctx: t.Context(), root: root, limits: DefaultLimits(), query: "fixture", queryLower: "fixture"}
	require.NoError(t, state.scanFile("vanishing.txt", info))
	require.Empty(t, state.chunks)
}
