package utils

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

func TestDirSize(t *testing.T) {
	// size, err := DirSize("/Users/laisky/Projects/go/src/pateo.com/go-fluentd")
	size, err := DirSize(".")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("size: %v", size)
	// t.Error()
}

func ExampleDirSize() {
	dirPath := "."
	size, err := DirSize(dirPath)
	if err != nil {
		Logger.Error("get dir size", zap.Error(err), zap.String("path", dirPath))
	}
	Logger.Info("got size", zap.Int64("size", size), zap.String("path", dirPath))
}

func TestCopyFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestCopyFile")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	if err = Logger.ChangeLevel(LoggerLevelDebug); err != nil {
		t.Fatal(err)
	}

	raw := []byte("fj2ojf392f2jflwejf92f93fu2o3jf32;fwjf")
	src := filepath.Join(dir, "src")
	srcFp, err := os.OpenFile(src, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	defer srcFp.Close()

	if _, err = srcFp.Write(raw); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	if err = CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	if err = srcFp.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(raw, got) {
		t.Fatalf("got %s", string(got))
	}

	if got, err = ioutil.ReadFile(src); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(raw, got) {
		t.Fatalf("got %s", string(got))
	}
}

func TestMoveFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "TestMoveFile")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	err = Logger.ChangeLevel(LoggerLevelDebug)
	require.NoError(t, err)

	raw := []byte("fj2ojf392f2jflwejf92f93fu2o3jf32;fwjf")
	src := filepath.Join(dir, "src")
	srcFp, err := os.OpenFile(src, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	defer srcFp.Close()

	if _, err = srcFp.Write(raw); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	err = MoveFile(src, dst)
	require.NoError(t, err)
	err = MoveFile(src, dst)
	require.Error(t, err)
	err = CopyFile(src, dst)
	require.Error(t, err)

	err = srcFp.Close()
	require.NoError(t, err)

	got, err := ioutil.ReadFile(dst)
	require.NoError(t, err)

	if !bytes.Equal(raw, got) {
		t.Fatalf("got %s", string(got))
	}

	if _, err = os.Stat(src); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestIsDirWritable(t *testing.T) {
	dir, err := ioutil.TempDir("", "fs")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	dirWritable := filepath.Join(dir, "writable")
	if err = os.Mkdir(dirWritable, os.ModePerm|os.ModeDir); err != nil {
		t.Fatalf("mkdir %+v", err)
	}

	dirNotWritable := filepath.Join(dir, "notwritable")
	if err = os.Mkdir(dirNotWritable, os.FileMode(0444)|os.ModeDir); err != nil {
		t.Fatalf("mkdir %+v", err)
	}

	if err := IsDirWritable(dirWritable); err != nil {
		t.Fatalf("%+v", err)
	}

	if err := IsDirWritable(dirNotWritable); err == nil {
		t.Fatal()
	}
}

func TestIsDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "fs")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	// case: not exist
	{
		ok, err := IsDir(filepath.Join(dir, "notexist"))
		require.False(t, ok)
		require.Error(t, err)
	}

	// case: exist
	{
		ok, err := IsDir(dir)
		require.True(t, ok)
		require.NoError(t, err)
	}
}

func TestListFilesInDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "fs")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	err = os.MkdirAll(filepath.Join(dir, "dir1", "dir2"), os.ModePerm)
	require.NoError(t, err)

	_, err = os.OpenFile(filepath.Join(dir, "dir1", "file1"), os.O_CREATE, os.ModePerm)
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
