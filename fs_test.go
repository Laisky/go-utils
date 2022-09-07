package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v2/log"
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
		log.Shared.Error("get dir size", zap.Error(err), zap.String("path", dirPath))
	}
	log.Shared.Info("got size", zap.Int64("size", size), zap.String("path", dirPath))
}

func TestCopyFile(t *testing.T) {
	t.Run("not exist", func(t *testing.T) {
		err := CopyFile(RandomStringWithLength(5), RandomStringWithLength(5))
		require.Error(t, err)
	})

	dir, err := ioutil.TempDir("", "TestCopyFile")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	if err = log.Shared.ChangeLevel(log.LevelDebug); err != nil {
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

	err = log.Shared.ChangeLevel(log.LevelDebug)
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

func TestNewTmpFileForContent(t *testing.T) {
	cnt := "yahoo"

	path, err := NewTmpFileForContent([]byte(cnt))
	require.NoError(t, err)

	got, err := ioutil.ReadFile(path)
	require.NoError(t, err)

	require.Equal(t, cnt, string(got))
}

// BenchmarkFileSHA1/md5_1MB-16         	     464	   2682812 ns/op	    4296 B/op	       7 allocs/op
// BenchmarkFileSHA1/sha1_1MB-16        	     548	   2253516 ns/op	    4336 B/op	       7 allocs/op
func BenchmarkFileSHA1(b *testing.B) {
	fp, err := os.CreateTemp("", "*")
	PanicIfErr(err)
	defer os.Remove(fp.Name())
	_, err = fp.WriteString(RandomStringWithLength(1024 * 1024))
	PanicIfErr(err)
	PanicIfErr(fp.Close())
	b.Run("md5 1MB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := FileMD5(fp.Name())
			PanicIfErr(err)
		}
	})
	b.Run("sha1 1MB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := FileSHA1(fp.Name())
			PanicIfErr(err)
		}
	})
}

func TestWatchFileChanging(t *testing.T) {
	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)

	fpath1 := filepath.Join(dir, "1")
	fp1, err := os.OpenFile(fpath1, os.O_CREATE|os.O_RDWR, os.ModePerm)
	require.NoError(t, err)

	fpath2 := filepath.Join(dir, "2")
	fp2, err := os.OpenFile(fpath2, os.O_CREATE|os.O_RDWR, os.ModePerm)
	require.NoError(t, err)

	var evts []fsnotify.Event
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = WatchFileChanging(ctx, []string{fpath1, fpath2}, func(e fsnotify.Event) {
		mu.Lock()
		defer mu.Unlock()

		evts = append(evts, e)
	})
	require.NoError(t, err)

	// wait wather start
	time.Sleep(200 * time.Millisecond)

	_, err = fp1.WriteString("yo")
	require.NoError(t, err)
	require.NoError(t, fp1.Close())

	_, err = fp2.WriteString("yo")
	require.NoError(t, err)
	require.NoError(t, fp2.Close())

	for {
		mu.Lock()
		l := len(evts)
		mu.Unlock()

		if l >= 2 {
			break
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}

	var got []fsnotify.Event
	mu.Lock()
	got = append(got, evts...)
	mu.Unlock()

	require.Equal(t, got[0].Op, fsnotify.Write)
	require.Equal(t, got[1].Op, fsnotify.Write)
	require.Contains(t, []string{fpath1, fpath2}, got[0].Name)
	require.Contains(t, []string{fpath1, fpath2}, got[1].Name)

	t.Run("delete file", func(t *testing.T) {
		mu.Lock()
		l := len(evts)
		mu.Unlock()

		require.NoError(t, os.Remove(fpath1))

		fp1, err := os.OpenFile(fpath1, os.O_RDWR|os.O_CREATE, 0o644)
		require.NoError(t, err)

		_, err = fp1.Write([]byte(RandomStringWithLength(10)))
		require.NoError(t, err)

		require.NoError(t, fp1.Close())

		time.Sleep(1500 * time.Millisecond)
		mu.Lock()
		require.Greater(t, len(evts), l)
		mu.Unlock()
	})
}

func TestFileMD5(t *testing.T) {
	t.Run("file not exist", func(t *testing.T) {
		_, err := FileMD5(RandomStringWithLength(10))
		require.Error(t, err)
	})

	cnt := []byte(RandomStringWithLength(10))
	t.Logf("write: %s", string(cnt))
	hasher := md5.New()
	_, err := hasher.Write(cnt)
	require.NoError(t, err)

	hashed := hex.EncodeToString(hasher.Sum(nil))
	fpath, err := NewTmpFileForContent(cnt)
	require.NoError(t, err)
	defer os.Remove(fpath)

	fhashed, err := FileMD5(fpath)
	require.NoError(t, err)
	require.Equal(t, hashed, fhashed)

	fp, err := os.OpenFile(fpath, os.O_APPEND|os.O_RDWR, 0o644)
	require.NoError(t, err)
	fp.Write([]byte(RandomStringWithLength(10)))
	require.NoError(t, fp.Close())

	fhashed2, err := FileMD5(fpath)
	require.NoError(t, err)
	require.NotEqual(t, fhashed, fhashed2)
}

func TestIsFileATimeChanged(t *testing.T) {
	t.Run("file not exist", func(t *testing.T) {
		_, _, err := IsFileATimeChanged(RandomStringWithLength(10), time.Now())
		require.Error(t, err)
	})

	fp, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer os.Remove(fp.Name())
	require.NoError(t, fp.Close())

	fi, err := os.Stat(fp.Name())
	require.NoError(t, err)
	atime := fi.ModTime()

	time.Sleep(time.Second)

	err = ioutil.WriteFile(fp.Name(), []byte(RandomStringWithLength(10)), 0o644)
	require.NoError(t, err)

	fi, err = os.Stat(fp.Name())
	require.NoError(t, err)
	require.False(t, atime.Equal(fi.ModTime()))

	time.Sleep(time.Second)
	changed, newATime, err := IsFileATimeChanged(fp.Name(), atime)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, newATime.Equal(fi.ModTime()))
}

func TestFileSHA1(t *testing.T) {
	fp, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer os.Remove(fp.Name())

	_, err = fp.WriteString("fwefjwefjwekjfweklfjwkl")
	require.NoError(t, err)
	require.NoError(t, fp.Close())

	hashed, err := FileSHA1(fp.Name())
	require.NoError(t, err)
	require.Equal(t, "2c4dee26eca505ebd8afdad00e417efa5e5e1290", hashed)

}
