package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
)

func Test_switch(t *testing.T) {
	v := "a"
	switch v {
	case "a", "b", "c":
		v = "c"
	default:
		v = "213"
	}

	require.Equal(t, v, "c")
}

func Test_removeDuplicate(t *testing.T) {
	dir, err := os.MkdirTemp("", "removeDuplicate*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// prepare files in dir
	err = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("123"), 0600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("123"), 0600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("1234"), 0600)
	require.NoError(t, err)

	// prepare files in dir/laisky
	dir2 := filepath.Join(dir, "laisky")
	require.NoError(t, os.MkdirAll(dir2, 0777))
	err = os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("123"), 0600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("123"), 0600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir2, "c.txt"), []byte("1234"), 0600)
	require.NoError(t, err)

	err = removeDuplicate(false, dir)
	require.NoError(t, err)

	ok, err := gutils.FileExists(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = gutils.FileExists(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = gutils.FileExists(filepath.Join(dir, "c.txt"))
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = gutils.FileExists(filepath.Join(dir2, "a.txt"))
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = gutils.FileExists(filepath.Join(dir2, "b.txt"))
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = gutils.FileExists(filepath.Join(dir2, "c.txt"))
	require.NoError(t, err)
	require.False(t, ok)
}
