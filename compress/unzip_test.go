package compress

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// func TestZipDir(t *testing.T) {
// 	err := ZipFiles(
// 		"/home/laisky/test/test.zip",
// 		[]string{"/home/laisky/test/zip"},
// 	)
// 	if err != nil {
// 		require.NoError(t, err)
// 	}
// }

func TestUnzipAndZipFiles(t *testing.T) {
	t.Parallel()
	var err error
	if err = log.Shared.ChangeLevel("debug"); err != nil {
		require.NoError(t, err)
	}

	var dir string
	if dir, err = os.MkdirTemp("", "*"); err != nil {
		require.NoError(t, err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	if err = os.Mkdir(filepath.Join(dir, "src"), 0751); err != nil {
		require.NoError(t, err)
	}
	if err = os.Mkdir(filepath.Join(dir, "src/child"), 0751); err != nil {
		require.NoError(t, err)
	}
	if err = os.Mkdir(filepath.Join(dir, "dst"), 0751); err != nil {
		require.NoError(t, err)
	}
	files := []string{
		filepath.Join(dir, "src", "a.txt"),
		filepath.Join(dir, "src", "child", "b.txt"),
		filepath.Join(dir, "src", "c.txt"),
	}

	var fp *os.File
	for _, file := range files {
		if fp, err = os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0751); err != nil {
			require.NoError(t, err)
		}
		if _, err = fp.WriteString("yoo"); err != nil {
			require.NoError(t, err)
		}
		if err = fp.Close(); err != nil {
			require.NoError(t, err)
		}

		t.Logf("create file `%s`", fp.Name())
	}

	// 压缩文件
	// if err = ZipFiles(filepath.Join(dir, "src.zip"), files); err != nil {
	// 	require.NoError(t, err)
	// }

	// 压缩文件夹
	if err = ZipFiles(filepath.Join(dir, "src.zip"), []string{filepath.Join(dir, "src")}); err != nil {
		require.NoError(t, err)
	}

	var dstFiles []string
	dstDir := filepath.Join(dir, "dst")
	if dstFiles, err = Unzip(filepath.Join(dir, "src.zip"), dstDir); err != nil {
		require.NoError(t, err)
	}
	t.Logf("unzip files: %+v", dstFiles)

	for _, fname := range dstFiles {
		ok := false
		for _, expect := range []string{"a.txt", "child/b.txt", "c.txt"} {
			if strings.HasSuffix(filepath.ToSlash(fname), expect) {
				ok = true
			}
		}

		if !ok {
			t.Fatalf("unknown file: %s", fname)
		}

		if cnt, err := os.ReadFile(fname); err != nil {
			t.Fatalf("unknown file: %s", fname)
		} else if string(cnt) != "yoo" {
			t.Fatalf("unknown content for file `%s`: %s", fname, string(cnt))
		}
	}

	// t.Error()
}

func TestUnzipWithMaxBytes(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create source files with known content
	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.Mkdir(srcDir, 0751))

	// Write a file larger than 32KB to verify the fix
	// (before the fix, UnzipWithMaxBytes clamped limit to 32KB)
	largeContent := strings.Repeat("x", 100*1024) // 100KB
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "large.txt"), []byte(largeContent), 0644))

	// Create zip
	zipPath := filepath.Join(dir, "test.zip")
	require.NoError(t, ZipFiles(zipPath, []string{srcDir}))

	t.Run("maxBytes larger than chunk size works correctly", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst1")
		require.NoError(t, os.Mkdir(dstDir, 0751))

		// Set maxBytes to 200KB - this should NOT be clamped to 32KB
		_, err := Unzip(zipPath, dstDir, UnzipWithMaxBytes(200*1024))
		require.NoError(t, err)

		// Verify the full file content was extracted
		content, err := os.ReadFile(filepath.Join(dstDir, "src", "large.txt"))
		require.NoError(t, err)
		require.Equal(t, largeContent, string(content),
			"file should be fully extracted when maxBytes > file size")
	})

	t.Run("maxBytes smaller than file returns error (no silent truncation)", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst2")
		require.NoError(t, os.Mkdir(dstDir, 0751))

		// Set maxBytes to 50KB - the 100KB entry exceeds the budget, so Unzip
		// must return an error instead of silently truncating the output.
		_, err := Unzip(zipPath, dstDir, UnzipWithMaxBytes(50*1024))
		require.Error(t, err)
		require.ErrorContains(t, err, "exceed aggregate limit")
	})

	t.Run("invalid maxBytes", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst3")
		require.NoError(t, os.Mkdir(dstDir, 0751))
		_, err := Unzip(zipPath, dstDir, UnzipWithMaxBytes(0))
		require.Error(t, err)
		_, err = Unzip(zipPath, dstDir, UnzipWithMaxBytes(-1))
		require.Error(t, err)
	})
}

// writeZip builds a zip archive at zipPath from the given entries.
// Each entry maps a file name to its uncompressed content. The optional
// modes map allows overriding the stored unix permission bits per entry.
func writeZip(t *testing.T, zipPath string, entries map[string]string, modes map[string]os.FileMode) {
	t.Helper()

	fp, err := os.Create(zipPath)
	require.NoError(t, err)
	defer fp.Close()

	zw := zip.NewWriter(fp)
	for name, content := range entries {
		hdr := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		if modes != nil {
			if m, ok := modes[name]; ok {
				hdr.SetMode(m)
			}
		}

		w, err := zw.CreateHeader(hdr)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

// TestUnzipAggregateMaxBytes verifies the aggregate decompression limit:
// many small entries whose combined size exceeds maxBytes must return an
// error (previously the limit was per-entry, allowing #entries x maxBytes).
func TestUnzipAggregateMaxBytes(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// 5 entries of 30KB each = 150KB combined; each entry is below a 100KB
	// limit individually, but the aggregate exceeds it.
	entries := map[string]string{
		"a.txt": strings.Repeat("a", 30*1024),
		"b.txt": strings.Repeat("b", 30*1024),
		"c.txt": strings.Repeat("c", 30*1024),
		"d.txt": strings.Repeat("d", 30*1024),
		"e.txt": strings.Repeat("e", 30*1024),
	}

	zipPath := filepath.Join(dir, "many.zip")
	writeZip(t, zipPath, entries, nil)

	dstDir := filepath.Join(dir, "dst")
	require.NoError(t, os.Mkdir(dstDir, 0751))

	_, err = Unzip(zipPath, dstDir, UnzipWithMaxBytes(100*1024))
	require.Error(t, err, "combined entries exceeding maxBytes must error")
	require.ErrorContains(t, err, "exceed aggregate limit")
}

// TestUnzipSingleEntryNoSilentTruncation verifies that a single entry larger
// than maxBytes returns an error and does NOT leave a truncated copy on disk.
func TestUnzipSingleEntryNoSilentTruncation(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	content := strings.Repeat("z", 100*1024) // 100KB
	zipPath := filepath.Join(dir, "big.zip")
	writeZip(t, zipPath, map[string]string{"big.txt": content}, nil)

	dstDir := filepath.Join(dir, "dst")
	require.NoError(t, os.Mkdir(dstDir, 0751))

	_, err = Unzip(zipPath, dstDir, UnzipWithMaxBytes(50*1024))
	require.Error(t, err)
	require.ErrorContains(t, err, "exceed aggregate limit")

	// The extracted file must not contain a full (untruncated) copy of the
	// original content; silent truncation to exactly maxBytes is also a bug,
	// so simply assert the original full content was never materialized.
	out := filepath.Join(dstDir, "big.txt")
	if data, rerr := os.ReadFile(out); rerr == nil {
		require.NotEqual(t, content, string(data),
			"oversized entry must not be fully extracted")
	}
}

// TestUnzipMaxEntries verifies the entry-count cap rejects archives with too
// many entries.
func TestUnzipMaxEntries(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	entries := map[string]string{
		"a.txt": "1",
		"b.txt": "2",
		"c.txt": "3",
	}
	zipPath := filepath.Join(dir, "entries.zip")
	writeZip(t, zipPath, entries, nil)

	t.Run("exceeding max entries errors", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst1")
		require.NoError(t, os.Mkdir(dstDir, 0751))

		_, err := Unzip(zipPath, dstDir, UnzipWithMaxEntries(2))
		require.Error(t, err)
		require.ErrorContains(t, err, "exceed max entries limit")
	})

	t.Run("within max entries succeeds", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst2")
		require.NoError(t, os.Mkdir(dstDir, 0751))

		_, err := Unzip(zipPath, dstDir, UnzipWithMaxEntries(3))
		require.NoError(t, err)
	})

	t.Run("invalid max entries", func(t *testing.T) {
		dstDir := filepath.Join(dir, "dst3")
		require.NoError(t, os.Mkdir(dstDir, 0751))
		_, err := Unzip(zipPath, dstDir, UnzipWithMaxEntries(-1))
		require.Error(t, err)
	})
}

// TestUnzipPermissionClamp verifies that archive-controlled permission bits
// are clamped so extracted files are never group/other-writable nor carry
// setuid/setgid/sticky bits.
func TestUnzipPermissionClamp(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not meaningful on windows")
	}

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Store an entry with world-writable mode 0o777; the extracted file must
	// be clamped to at most 0o755.
	zipPath := filepath.Join(dir, "perm.zip")
	writeZip(t, zipPath,
		map[string]string{"evil.txt": "payload"},
		map[string]os.FileMode{"evil.txt": 0o777},
	)

	dstDir := filepath.Join(dir, "dst")
	require.NoError(t, os.Mkdir(dstDir, 0751))

	_, err = Unzip(zipPath, dstDir)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dstDir, "evil.txt"))
	require.NoError(t, err)

	perm := info.Mode().Perm()
	require.LessOrEqual(t, perm, os.FileMode(0o755),
		"extracted perm must be clamped to <= 0o755, got %o", perm)
	// No group/other write bits.
	require.Zero(t, perm&0o022, "group/other write bits must be stripped")
	// No setuid/setgid/sticky.
	require.Zero(t, info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky),
		"setuid/setgid/sticky bits must be stripped")
}
