package fileguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestOverwriteInPlaceFailurePreservesInode checks the documented partial-write
// contract: an in-place failure may change content but never replaces the inode.
func TestOverwriteInPlaceFailurePreservesInode(t *testing.T) {
	name := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.WriteFile(name, []byte("original contents"), 0644))
	before, err := os.Stat(name)
	require.NoError(t, err)
	require.ErrorIs(t, OverwriteInPlace(name, os.O_TRUNC, 0600, nil, failingWrite), errInjected)
	after, err := os.Stat(name)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, "partial", string(data))
}

// TestOverwriteInPlaceAppendFlag preserves caller-requested append semantics
// without truncating an existing file when O_TRUNC is not present.
func TestOverwriteInPlaceAppendFlag(t *testing.T) {
	name := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.WriteFile(name, []byte("before"), 0600))
	err := OverwriteInPlace(name, os.O_APPEND, 0600, nil, func(f *os.File) error {
		_, err := f.WriteString("after")
		return errors.Wrap(err, "append fixture")
	})
	require.NoError(t, err)
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, "beforeafter", string(data))
}
