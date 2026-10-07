package utils

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

func TestMoveFile(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestMoveFile-*")
	require.NoError(t, err)
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	err = log.Shared.ChangeLevel(log.LevelDebug)
	require.NoError(t, err)

	raw := []byte("fj2ojf392f2jflwejf92f93fu2o3jf32;fwjf")
	src := filepath.Join(dir, "src")
	srcFp, err := os.OpenFile(src, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)
	defer srcFp.Close()

	_, err = srcFp.Write(raw)
	require.NoError(t, err)

	dst := filepath.Join(dir, "dst")
	err = MoveFile(src, dst)
	require.NoError(t, err)
	err = MoveFile(src, dst)
	require.Error(t, err)
	err = CopyFile(src, dst)
	require.Error(t, err)

	err = srcFp.Close()
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)

	require.Equal(t, raw, got)

	_, err = os.Stat(src)
	require.True(t, os.IsNotExist(err))
}

func TestReplaceFile(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestReplaceFile-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	t.Run("replace exists file", func(t *testing.T) {
		fpath := filepath.Join(dir, "fpath")
		err := os.WriteFile(fpath, []byte(RandomStringWithLength(432)), 0600)
		require.NoError(t, err)

		cnt, err := RandomBytesWithLength(1024 * 1024)
		require.NoError(t, err)
		err = ReplaceFile(fpath, cnt, 0640)
		require.NoError(t, err)

		finfo, err := os.Stat(fpath)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0640), finfo.Mode())

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})

	t.Run("replace non-exists file", func(t *testing.T) {
		fpath := filepath.Join(dir, "nonexists")
		cnt, err := RandomBytesWithLength(1024 * 1024)
		require.NoError(t, err)

		err = ReplaceFile(fpath, cnt, 0640)
		require.NoError(t, err)

		finfo, err := os.Stat(fpath)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0640), finfo.Mode())

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})
}

func TestReplaceFileStream(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	dst := filepath.Join(dir, "dst")
	err = os.WriteFile(dst, []byte(RandomStringWithLength(432)), 0600)
	require.NoError(t, err)

	src := filepath.Join(dir, "src")
	cnt, err := RandomBytesWithLength(1024 * 1024)
	require.NoError(t, err)

	err = os.WriteFile(src, cnt, 0644)
	require.NoError(t, err)

	srcfp, err := os.Open(src)
	require.NoError(t, err)
	defer srcfp.Close()

	err = ReplaceFileStream(dst, srcfp, 0640)
	require.NoError(t, err)

	finfo, err := os.Stat(dst)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), finfo.Mode())

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, cnt, got)
}

// TestReplaceFileAtomicExclSwap is a regression test for the O_EXCL hardening:
// the swap file is created with O_CREATE|O_EXCL so it refuses to follow a
// pre-existing file/symlink at the swap path. This verifies legitimate
// create/replace behavior is unaffected by that change.
func TestReplaceFileAtomicExclSwap(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestReplaceFileAtomicExclSwap-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	t.Run("ReplaceFile creates new file", func(t *testing.T) {
		fpath := filepath.Join(dir, "create")
		cnt := []byte("brand-new-content")
		require.NoError(t, ReplaceFile(fpath, cnt, 0640))

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})

	t.Run("ReplaceFile replaces existing file", func(t *testing.T) {
		fpath := filepath.Join(dir, "replace")
		require.NoError(t, os.WriteFile(fpath, []byte("old"), 0600))

		cnt := []byte("replaced-content")
		require.NoError(t, ReplaceFile(fpath, cnt, 0640))

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})

	t.Run("ReplaceFileAtomic creates and replaces", func(t *testing.T) {
		fpath := filepath.Join(dir, "atomic")
		require.NoError(t, os.WriteFile(fpath, []byte("old"), 0600))

		cnt := []byte("atomic-replaced-content")
		require.NoError(t, ReplaceFileAtomic(fpath, io.NopCloser(bytes.NewReader(cnt)), 0640))

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})
}
