package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestSearchCompatibilityResultShape preserves [] for an existing empty result and nil for absent roots.
func TestSearchCompatibilityResultShape(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(Config{RootDir: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.Write(t.Context(), "demo", "/docs/a.txt", "ordinary content", memorystorage.WriteModeTruncate, 0))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "demo", "empty"), 0700))
	for _, tc := range []struct{ name, project, prefix, json string }{
		{"no_match", "demo", "/docs", "[]"},
		{"empty_directory", "demo", "/empty", "[]"},
		{"missing_prefix", "demo", "/missing", "null"},
		{"missing_project", "absent", "/", "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := engine.Search(t.Context(), tc.project, "not present", tc.prefix, 1)
			require.NoError(t, err)
			encoded, err := json.Marshal(chunks)
			require.NoError(t, err)
			require.Equal(t, tc.json, string(encoded))
		})
	}
}
