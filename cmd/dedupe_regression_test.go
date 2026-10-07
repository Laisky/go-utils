package cmd

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeDedupeFile writes content to dir/name. It takes the test handle, the
// directory, the file name and the content, and returns the file path.
func writeDedupeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// requireDedupeContent asserts that path exists and holds want.
// It takes the test handle, the file path and the expected content.
func requireDedupeContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(data))
}

// syntheticCollisionCache prepopulates a digest cache so that candidate's real
// SHA-1 maps to a different kept file, emulating a digest collision. It takes the
// test handle, the kept path, its recorded size and the candidate content, and
// returns the cache.
func syntheticCollisionCache(t *testing.T, kept string, keptSize int64, candidate string) *sync.Map {
	t.Helper()
	sum := sha1.Sum([]byte(candidate))
	digest := hex.EncodeToString(sum[:])
	cache := &sync.Map{}
	cache.Store(digest, &dupFile{path: kept, hash: digest, sizeBytes: keptSize})
	return cache
}

// TestCheckDupByHashSyntheticCollision verifies that a digest match alone never
// deletes a file with different bytes; regression for issue #52.
func TestCheckDupByHashSyntheticCollision(t *testing.T) {
	cases := map[string][2]string{
		"equal-size-different-bytes": {"AAAA", "BBBB"},
		"different-size":             {"AAAA", "BBBBBB"},
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			kept := writeDedupeFile(t, dir, "kept.txt", contents[0])
			candidate := writeDedupeFile(t, dir, "candidate.txt", contents[1])
			cache := syntheticCollisionCache(t, kept, int64(len(contents[0])), contents[1])

			deleted, err := checkDupByHash(false, cache, candidate)
			require.NoError(t, err)
			require.False(t, deleted)
			requireDedupeContent(t, kept, contents[0])
			requireDedupeContent(t, candidate, contents[1])
		})
	}
}

// TestCheckDupByHashSizeMismatchRecorded verifies that a stale cached size that
// differs from the kept file still preserves both files; regression for issue #52.
func TestCheckDupByHashSizeMismatchRecorded(t *testing.T) {
	dir := t.TempDir()
	kept := writeDedupeFile(t, dir, "kept.txt", "SAME")
	candidate := writeDedupeFile(t, dir, "candidate.txt", "SAME")
	cache := syntheticCollisionCache(t, kept, 99, "SAME")

	deleted, err := checkDupByHash(false, cache, candidate)
	require.NoError(t, err)
	require.False(t, deleted)
	requireDedupeContent(t, candidate, "SAME")
}

// TestCheckDupByHashReadErrorPreserves verifies that a comparison failure, such as
// a vanished kept file, preserves the candidate; regression for issue #52.
func TestCheckDupByHashReadErrorPreserves(t *testing.T) {
	dir := t.TempDir()
	candidate := writeDedupeFile(t, dir, "candidate.txt", "DATA")
	cache := syntheticCollisionCache(t, filepath.Join(dir, "missing.txt"), 4, "DATA")

	deleted, err := checkDupByHash(false, cache, candidate)
	require.Error(t, err)
	require.False(t, deleted)
	requireDedupeContent(t, candidate, "DATA")
}

// TestCheckDupByHashTrueDuplicate verifies that byte-identical duplicates are
// still removed, and only reported in dry-run mode; regression for issue #52.
func TestCheckDupByHashTrueDuplicate(t *testing.T) {
	for _, dry := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "dry"}[dry], func(t *testing.T) {
			dir := t.TempDir()
			first := writeDedupeFile(t, dir, "first.txt", "DUPLICATE")
			second := writeDedupeFile(t, dir, "second.txt", "DUPLICATE")
			cache := &sync.Map{}

			deleted, err := checkDupByHash(dry, cache, first)
			require.NoError(t, err)
			require.False(t, deleted)
			deleted, err = checkDupByHash(dry, cache, second)
			require.NoError(t, err)
			require.True(t, deleted)
			requireDedupeContent(t, first, "DUPLICATE")
			_, err = os.Stat(second)
			if dry {
				require.NoError(t, err, "dry run must not delete")
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

// md5Target returns the content-addressed destination md5dir computes for content.
// It takes the target directory, the content and the lowercase extension.
func md5Target(targetDir, content, ext string) string {
	sum := md5.Sum([]byte(content))
	digest := hex.EncodeToString(sum[:])
	return filepath.Join(targetDir, digest[:2], digest+ext)
}

// runMd5DirCommand runs the md5dir command with the given directories and mode.
// It takes the test handle, source and target directories and the remain flag,
// and returns the command error.
func runMd5DirCommand(t *testing.T, source, target string, remain bool) error {
	t.Helper()
	saved, savedCaption := md5DirArg, md5DirSaveCaption
	t.Cleanup(func() { md5DirArg, md5DirSaveCaption = saved, savedCaption })
	md5DirSaveCaption = func(context.Context, string, string) error { return nil }
	md5DirArg.SourceDir, md5DirArg.TargetDir, md5DirArg.RemainSource = source, target, remain
	return md5DirCMD.RunE(md5DirCMD, nil)
}

// md5DirModes enumerates move and copy (--remain) modes.
var md5DirModes = map[string]bool{"move": false, "copy": true}

// TestMd5DirExistingDifferentDestination verifies that md5dir never replaces an
// existing destination holding different bytes; regression for issue #52.
func TestMd5DirExistingDifferentDestination(t *testing.T) {
	for mode, remain := range md5DirModes {
		t.Run(mode, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			src := writeDedupeFile(t, source, "photo.JPG", "SOURCE")
			dst := md5Target(target, "SOURCE", ".jpg")
			require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o700))
			require.NoError(t, os.WriteFile(dst, []byte("COLLIDING-OTHER"), 0o600))

			err := runMd5DirCommand(t, source, target, remain)
			require.Error(t, err)
			requireDedupeContent(t, dst, "COLLIDING-OTHER")
			requireDedupeContent(t, src, "SOURCE")
		})
	}
}

// TestMd5DirExistingIdenticalDestination verifies that identical content is
// deduplicated: move removes the redundant source, copy keeps it; regression for issue #52.
func TestMd5DirExistingIdenticalDestination(t *testing.T) {
	for mode, remain := range md5DirModes {
		t.Run(mode, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			src := writeDedupeFile(t, source, "photo.jpg", "SAME")
			dst := md5Target(target, "SAME", ".jpg")
			require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o700))
			require.NoError(t, os.WriteFile(dst, []byte("SAME"), 0o600))

			require.NoError(t, runMd5DirCommand(t, source, target, remain))
			requireDedupeContent(t, dst, "SAME")
			_, err := os.Stat(src)
			if remain {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

// TestMd5DirFreshDestination verifies the ordinary move and copy paths; regression for issue #52.
func TestMd5DirFreshDestination(t *testing.T) {
	for mode, remain := range md5DirModes {
		t.Run(mode, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			src := writeDedupeFile(t, source, "a.TXT", "ALPHA")
			writeDedupeFile(t, source, "b.txt", "BRAVO")

			require.NoError(t, runMd5DirCommand(t, source, target, remain))
			requireDedupeContent(t, md5Target(target, "ALPHA", ".txt"), "ALPHA")
			requireDedupeContent(t, md5Target(target, "BRAVO", ".txt"), "BRAVO")
			_, err := os.Stat(src)
			if remain {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

// TestMd5DirAlreadyInPlace verifies that a file already stored at its computed
// destination is left intact in both modes; regression for issue #52.
func TestMd5DirAlreadyInPlace(t *testing.T) {
	for mode, remain := range md5DirModes {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			sum := md5.Sum([]byte("INPLACE"))
			digest := hex.EncodeToString(sum[:])
			sub := filepath.Join(dir, digest[:2])
			require.NoError(t, os.Mkdir(sub, 0o700))
			path := writeDedupeFile(t, sub, digest+".txt", "INPLACE")

			require.NoError(t, runMd5DirCommand(t, sub, dir, remain))
			requireDedupeContent(t, path, "INPLACE")
			require.True(t, strings.HasSuffix(path, ".txt"))
		})
	}
}

// TestCompareFileContentMultiChunk verifies bounded streaming comparison across
// chunk boundaries, including a difference in the final byte; regression for issue #52.
func TestCompareFileContentMultiChunk(t *testing.T) {
	dir := t.TempDir()
	base := strings.Repeat("0123456789abcdef", 3*compareChunkBytes/16+3)
	first := writeDedupeFile(t, dir, "first.bin", base+"X")
	same := writeDedupeFile(t, dir, "same.bin", base+"X")
	different := writeDedupeFile(t, dir, "different.bin", base+"Y")

	cmp, err := compareFileContent(first, same)
	require.NoError(t, err)
	require.True(t, cmp.equal)
	cmp, err = compareFileContent(first, different)
	require.NoError(t, err)
	require.False(t, cmp.equal)

	_, err = compareFileContent(first, dir)
	require.Error(t, err, "directories are never treated as comparable files")
}

// TestRemoveIfUnchangedPreservesReplacedFile verifies that a file replaced after
// comparison is not deleted; regression for issue #52.
func TestRemoveIfUnchangedPreservesReplacedFile(t *testing.T) {
	dir := t.TempDir()
	first := writeDedupeFile(t, dir, "first.txt", "SAME")
	second := writeDedupeFile(t, dir, "second.txt", "SAME")
	cmp, err := compareFileContent(first, second)
	require.NoError(t, err)
	require.True(t, cmp.equal)

	require.NoError(t, os.Remove(second))
	writeDedupeFile(t, dir, "second.txt", "DIFFERENT")
	require.Error(t, removeIfUnchanged(second, cmp.second))
	requireDedupeContent(t, second, "DIFFERENT")
}
