package utils

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/zap"
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
	if err = MoveFile(src, dst); err != nil {
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

	if _, err = os.Stat(src); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
