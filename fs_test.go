package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

func TestDirSize(t *testing.T) {
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

func TestCopyFile(t *testing.T) {
	t.Run("not exist", func(t *testing.T) {
		err := CopyFile(RandomStringWithLength(5), RandomStringWithLength(5))
		require.Error(t, err)
	})

	dir, err := os.MkdirTemp("", "TestCopyFile*")
	require.NoError(t, err)
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	err = log.Shared.ChangeLevel(log.LevelDebug)
	require.NoError(t, err)

	src := filepath.Join(dir, "src")
	raw := []byte("fj2ojf392f2jflwejf92f93fu2o3jf32;fwjf")
	t.Run("prepare src file", func(t *testing.T) {
		srcFp, err := os.OpenFile(src, os.O_CREATE|os.O_RDWR, 0644)
		require.NoError(t, err)
		defer srcFp.Close()

		_, err = srcFp.Write(raw)
		require.NoError(t, err)
	})

	dst := filepath.Join(dir, "dst")
	t.Run("copy new file", func(t *testing.T) {
		err = CopyFile(src, dst)
		require.NoError(t, err)

		got, err := os.ReadFile(dst)
		require.NoError(t, err)

		if !bytes.Equal(raw, got) {
			require.NoError(t, err)
		}

		got, err = os.ReadFile(src)
		require.NoError(t, err)

		if !bytes.Equal(raw, got) {
			require.NoError(t, err)
		}
	})

	raw = []byte(RandomStringWithLength(100))
	t.Run("copy overwrite", func(t *testing.T) {

		err = CopyFile(src, dst)
		require.ErrorContains(t, err, "exists")

		fp, err := os.OpenFile(src, os.O_TRUNC|os.O_WRONLY, 0644)
		require.NoError(t, err)

		_, err = fp.Write(raw)
		require.NoError(t, err)
		require.NoError(t, fp.Close())

		// overwrite by new content
		err = CopyFile(src, dst, Overwrite())
		require.NoError(t, err)

		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		require.Equal(t, raw, got)
	})
}

func TestMoveFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestMoveFile*")
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

func TestIsDirWritable(t *testing.T) {
	dir, err := os.MkdirTemp("", "fs*")
	require.NoError(t, err)
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	dirWritable := filepath.Join(dir, "writable")
	err = os.Mkdir(dirWritable, 0751)
	require.NoError(t, err)

	dirNotWritable := filepath.Join(dir, "notwritable")
	err = os.Mkdir(dirNotWritable, 0751)
	require.NoError(t, err)

	err = IsDirWritable(dirWritable)
	require.NoError(t, err)

	err = IsDirWritable(dirNotWritable)
	require.NoError(t, err)
}

func TestIsDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "fs*")
	require.NoError(t, err)
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
	dir, err := os.MkdirTemp("", "fs*")
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

func TestNewTmpFileForContent(t *testing.T) {
	cnt := "yahoo"

	path, err := NewTmpFileForContent([]byte(cnt))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
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
	fp1, err := os.OpenFile(fpath1, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)

	fpath2 := filepath.Join(dir, "2")
	fp2, err := os.OpenFile(fpath2, os.O_CREATE|os.O_RDWR, 0644)
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

	err = os.WriteFile(fp.Name(), []byte(RandomStringWithLength(10)), 0644)
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

func TestFileExists(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestFileExists*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	t.Run("not exists", func(t *testing.T) {
		fpath := filepath.Join(dir, "laisky")
		ok, err := FileExists(fpath)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("is dir", func(t *testing.T) {
		fpath := filepath.Join(dir, "laisky")
		err := os.MkdirAll(fpath, 0700)
		require.NoError(t, err)

		ok, err := FileExists(fpath)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("is file", func(t *testing.T) {
		fpath := filepath.Join(dir, "laisky123")
		fp, err := os.Create(fpath)
		require.NoError(t, err)
		require.NoError(t, fp.Close())

		ok, err := FileExists(fpath)
		require.NoError(t, err)
		require.True(t, ok)
	})
}

func TestRenderTemplate(t *testing.T) {
	const tpl = `hello, {{.Name}}`
	arg := struct{ Name string }{"laisky"}

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fpath := filepath.Join(dir, "tpl")
	err = os.WriteFile(fpath, []byte(tpl), 0400)
	require.NoError(t, err)

	got, err := RenderTemplateFile(fpath, arg)
	require.NoError(t, err)
	require.Equal(t, "hello, laisky", string(got))
}

func TestReplaceFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "*")
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

func TestFilepathJoin(t *testing.T) {
	type args struct {
		paths []string
	}
	tests := []struct {
		name       string
		args       args
		wantResult string
		err        string
	}{
		{"0", args{[]string{}}, "", "empty paths"},
		{"1", args{[]string{"a"}}, "a", ""},
		{"2", args{[]string{"a", "b"}}, "a/b", ""},
		{"3", args{[]string{"a", "b", "c"}}, "a/b/c", ""},
		{"4", args{[]string{"a", "b", "../c"}}, "a/c", ""},
		{"5", args{[]string{"a", "b", "../../c"}}, "c", "escaped dst"},
		{"6", args{[]string{"a", "b", "../../ab"}}, "ab", "escaped dst"},
		{"7", args{[]string{"", "b"}}, "b", ""},
		{"8", args{[]string{"", "b", "../c"}}, "c", "escaped dst"},
		{"9", args{[]string{"", "b", "../../c"}}, "../c", "escaped dst"},
		{"10", args{[]string{"", "", "b"}}, "b", ""},
		{"11", args{[]string{"", "", "b", "../c"}}, "c", "escaped dst"},
		{"12", args{[]string{"", "", "b", "../../c"}}, "../c", "escaped dst"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult, err := JoinFilepath(tt.args.paths...)
			if tt.err == "" {
				require.NoError(t, err, "[%s]", tt.name)
				require.Equal(t, tt.wantResult, gotResult, "[%s]", tt.name)
			} else {
				require.ErrorContains(t, err, tt.err, "[%s] %s", tt.name, gotResult)
			}
		})
	}
}
