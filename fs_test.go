package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// TestCopyFile verifies CopyFile end to end: copying a nonexistent source fails, copying to a new
// destination reproduces the source bytes and leaves the source intact, copying onto an existing
// destination without Overwrite fails with an "exists" error, and Overwrite replaces the destination
// with the source's updated content.
func TestCopyFile(t *testing.T) {
	t.Parallel()
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

// TestIsDirWritable verifies that IsDirWritable returns nil for a directory the caller may create files in and an
// error for one it may not (mode 0500), and that it leaves no probe file behind in either case. The negative case
// is skipped when running as root, which bypasses directory permissions, and on Windows, where a directory's mode
// bits do not prevent creating files in it.
func TestIsDirWritable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("writable", func(t *testing.T) {
		t.Parallel()
		dirWritable := filepath.Join(dir, "writable")
		require.NoError(t, os.Mkdir(dirWritable, 0o751))

		require.NoError(t, IsDirWritable(dirWritable))
		requireDirEmpty(t, dirWritable)
	})

	t.Run("not writable", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("directory mode bits do not restrict file creation on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory permissions, so no directory is unwritable")
		}

		dirNotWritable := filepath.Join(dir, "notwritable")
		require.NoError(t, os.Mkdir(dirNotWritable, 0o700))
		require.NoError(t, os.Chmod(dirNotWritable, 0o500))
		t.Cleanup(func() {
			// Restore write permission so t.TempDir can remove the tree, even if an assertion failed.
			require.NoError(t, os.Chmod(dirNotWritable, 0o700))
		})

		err := IsDirWritable(dirNotWritable)
		require.Error(t, err)
		require.ErrorIs(t, err, os.ErrPermission)
		requireDirEmpty(t, dirNotWritable)
	})
}

// requireDirEmpty fails the test unless dir exists and contains no entries, which proves that IsDirWritable
// removed (or never created) its probe file. It takes the test handle and the directory path and returns nothing.
func requireDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	require.Empty(t, names, "IsDirWritable must not leave files in %s", dir)
}

// TestIsDir verifies that IsDir returns false with an error for a nonexistent path and true without an
// error for an existing directory.
func TestIsDir(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestIsDir-*")
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

// TestNewTmpFileForContent verifies that NewTmpFileForContent writes the given bytes to a new temporary
// file and returns a path whose content reads back unchanged.
func TestNewTmpFileForContent(t *testing.T) {
	t.Parallel()
	cnt := "yahoo"

	path, err := NewTmpFileForContent([]byte(cnt))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	require.Equal(t, cnt, string(got))
}

// BenchmarkFileSHA1 measures the per-call cost of FileMD5 and FileSHA1 when hashing a 1 MiB temporary
// file.
//
// BenchmarkFileSHA1/md5_1MB-16         	     464	   2682812 ns/op	    4296 B/op	       7 allocs/op
// BenchmarkFileSHA1/sha1_1MB-16        	     548	   2253516 ns/op	    4336 B/op	       7 allocs/op
func BenchmarkFileSHA1(b *testing.B) {
	fp, err := os.CreateTemp("", "BenchmarkFileSHA1-*")
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

// TestWatchFileChanging verifies that WatchFileChanging delivers fsnotify.Write events naming the
// watched files after both files are written, and that deleting and recreating a watched file with new
// content produces further events within 1.5 seconds.
func TestWatchFileChanging(t *testing.T) {
	t.Parallel()
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

// TestFileMD5 verifies that FileMD5 fails for a nonexistent path, returns the hex MD5 digest matching
// crypto/md5 for a temporary file's content, and returns a different digest after data is appended.
func TestFileMD5(t *testing.T) {
	t.Parallel()
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

// TestIsFileATimeChanged verifies that IsFileATimeChanged fails for a nonexistent path and, after the
// file is rewritten, reports changed=true together with the file's new modification time. Despite its
// name, the function compares modification times.
func TestIsFileATimeChanged(t *testing.T) {
	t.Parallel()
	t.Run("file not exist", func(t *testing.T) {
		_, _, err := IsFileATimeChanged(RandomStringWithLength(10), time.Now())
		require.Error(t, err)
	})

	fp, err := os.CreateTemp("", "TestIsFileATimeChanged-*")
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

// TestFileSHA1 verifies that FileSHA1 returns the expected lowercase hex SHA-1 digest of a temporary
// file holding a fixed string.
func TestFileSHA1(t *testing.T) {
	t.Parallel()
	fp, err := os.CreateTemp("", "TestFileSHA1-*")
	require.NoError(t, err)
	defer os.Remove(fp.Name())

	_, err = fp.WriteString("fwefjwefjwekjfweklfjwkl")
	require.NoError(t, err)
	require.NoError(t, fp.Close())

	hashed, err := FileSHA1(fp.Name())
	require.NoError(t, err)
	require.Equal(t, "2c4dee26eca505ebd8afdad00e417efa5e5e1290", hashed)

}

// TestFileExists verifies that FileExists returns (false, nil) for a missing path and for a directory,
// and (true, nil) for a regular file.
func TestFileExists(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestFileExists-*")
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

// TestRenderTemplate verifies that RenderTemplateFile reads a text/template file and renders it with
// the given struct argument, producing "hello, laisky".
func TestRenderTemplate(t *testing.T) {
	t.Parallel()
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

// TestFilepathJoin verifies JoinFilepath with table cases: an empty argument list is rejected, children
// are joined and cleaned beneath the base (including "../" that stays inside it), parent components
// that escape the base return an "escaped basedir" error, an empty first base is always rejected, and
// an absolute base is joined normally.
func TestFilepathJoin(t *testing.T) {
	t.Parallel()
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
		{"5", args{[]string{"a", "b", "../../c"}}, "c", "escaped basedir"},
		{"6", args{[]string{"a", "b", "../../ab"}}, "ab", "escaped basedir"},
		{"7", args{[]string{"", "b"}}, "", "trusted base path must not be empty"},
		{"8", args{[]string{"", "b", "../c"}}, "", "trusted base path must not be empty"},
		{"9", args{[]string{"", "b", "../../c"}}, "", "trusted base path must not be empty"},
		{"10", args{[]string{"", "", "b"}}, "", "trusted base path must not be empty"},
		{"11", args{[]string{"", "", "b", "../c"}}, "", "trusted base path must not be empty"},
		{"12", args{[]string{"", "", "b", "../../c"}}, "", "trusted base path must not be empty"},
		{"13", args{[]string{"/tmp", "285248985", ".fpath.swp-czfNpx"}}, "/tmp/285248985/.fpath.swp-czfNpx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult, err := JoinFilepath(tt.args.paths...)
			if tt.err == "" {
				require.NoError(t, err, "[%s]", tt.name)
				require.Equal(t, filepath.FromSlash(tt.wantResult), gotResult, "[%s]", tt.name)
			} else {
				require.ErrorContains(t, err, tt.err, "[%s] %s", tt.name, gotResult)
			}
		})
	}
}
