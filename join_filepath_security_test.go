package utils

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity62TrustedBase verifies that an empty trusted root never promotes a child.
func TestSecurity62TrustedBase(t *testing.T) {
	for _, input := range [][]string{nil, {""}, {"", ""}, {"", "../outside"}, {"", "safe"}} {
		result, err := JoinFilepath(input...)
		require.Error(t, err)
		require.Equal(t, "", result)
	}
	base := t.TempDir()
	for _, child := range []string{"../outside", "a/../../outside", filepath.Join(string(filepath.Separator), "outside")} {
		result, err := JoinFilepath(base, child)
		require.Error(t, err)
		require.Equal(t, "", result)
	}
	for _, parts := range [][]string{{base}, {base, "child"}, {base, "", "child"}, {base + string(filepath.Separator), "child"}, {base, "a/../child"}} {
		result, err := JoinFilepath(parts...)
		require.NoError(t, err)
		expected := base
		if len(parts) > 1 {
			expected = filepath.Join(base, "child")
		}
		require.Equal(t, expected, result)
	}
	got, err := JoinFilepath(".", "child")
	require.NoError(t, err)
	require.Equal(t, "child", got)
}
