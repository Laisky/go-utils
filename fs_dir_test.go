package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

func TestDirSize(t *testing.T) {
	t.Parallel()
	// size, err := DirSize("/Users/laisky/Projects/go/src/pateo.com/go-fluentd")
	size, err := DirSize(".")
	if err != nil {
		require.NoError(t, err)
	}
	t.Logf("size: %v", size)
	// t.Error()
}

func ExampleDirSize() {
	dirPath := "."
	size, err := DirSize(dirPath)
	if err != nil {
		log.Shared.Error("get dir size", zap.Error(err), zap.String("path", dirPath))
	}
	log.Shared.Info("got size", zap.Int64("size", size), zap.String("path", dirPath))
}

func TestListFilesInDir(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestListFilesInDir-*")
	require.NoError(t, err)
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	err = os.MkdirAll(filepath.Join(dir, "dir1", "dir2"), 0751)
	require.NoError(t, err)

	_, err = os.OpenFile(filepath.Join(dir, "dir1", "file1"), os.O_CREATE, 0644)
	require.NoError(t, err)

	// case: exist
	{
		files, err := ListFilesInDir(dir)
		require.NoError(t, err)
		require.Len(t, files, 0)

		files, err = ListFilesInDir(filepath.Join(dir, "dir1"))
		require.NoError(t, err)
		require.Len(t, files, 1)

		files, err = ListFilesInDir(filepath.Join(dir, "notexist"))
		require.Error(t, err)
		require.Len(t, files, 0)
	}
}
