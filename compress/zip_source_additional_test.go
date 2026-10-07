package compress

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity70CycleAndOutputOverlap rejects finite cycle/overlap fixtures without publication.
func TestSecurity70CycleAndOutputOverlap(t *testing.T) {
	parent := t.TempDir()
	selected := filepath.Join(parent, "selected")
	require.NoError(t, os.Mkdir(selected, 0700))
	link := filepath.Join(selected, "cycle")
	security70Link(t, ".", link)
	require.Error(t, ZipFiles(filepath.Join(parent, "cycle.zip"), []string{selected}))
	require.NoError(t, os.Remove(link))
	output := filepath.Join(selected, "self.zip")
	require.Error(t, ZipFiles(output, []string{selected}))
	entries, err := os.ReadDir(selected)
	require.NoError(t, err)
	require.Empty(t, entries, "staging must be removed")
}

// TestSecurity70ChangingSource rejects a size change after source metadata was captured.
func TestSecurity70ChangingSource(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "source")
	require.NoError(t, os.WriteFile(p, []byte("before"), 0600))
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	info, err := root.Lstat("source")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("grown since inspection"), 0600))
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	state := zipSourceState{writer: writer}
	require.Error(t, state.add(root, "source", "", info, 0))
	require.NoError(t, writer.Close())
}
