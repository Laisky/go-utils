package crypto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// filesTestKey is a fixed 16-byte raw AES key for directory tests.
var filesTestKey = []byte("0123456789abcdef")

// filesTestHook returns a package-test option installing hook.
func filesTestHook(hook func(ctx context.Context, stage aesFilesHookStage, name string) error) AESEncryptFilesInDirOption {
	return func(o *encryptFilesOption) error {
		o.hook = hook
		return nil
	}
}

// writeFilesTestDir creates dir/name for each entry of files and returns dir.
func writeFilesTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return dir
}

// requireNoEncOutputs asserts that dir holds no ".enc" outputs and no staging files.
func requireNoEncOutputs(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasSuffix(e.Name(), defaultEncryptSuffix), "unexpected output %s", e.Name())
		require.False(t, strings.HasPrefix(e.Name(), aesFilesTempPrefix), "leftover staging file %s", e.Name())
	}
}

// requireNoStagingFiles asserts that dir holds no leftover staging files.
func requireNoStagingFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasPrefix(e.Name(), aesFilesTempPrefix), "leftover staging file %s", e.Name())
	}
}

// TestAESFilesInDirSecurity_BoundedConcurrency checks that no more than the
// configured number of workers are active, with jobs held at a barrier until
// either every job arrived (unbounded pool) or a timeout. Regression for issue #65.
func TestAESFilesInDirSecurity_BoundedConcurrency(t *testing.T) {
	t.Parallel()

	const n, limit = 8, 2
	files := map[string]string{}
	for i := range n {
		files[fmt.Sprintf("f%d.txt", i)] = "data"
	}

	for _, tc := range []struct {
		name  string
		limit int
		opts  []AESEncryptFilesInDirOption
	}{
		{"explicit", limit, []AESEncryptFilesInDirOption{WithAESFilesInDirMaxConcurrency(limit)}},
		{"default", defaultAESFilesInDirMaxConcurrency, nil},
	} {
		dir := writeFilesTestDir(t, files)
		var mu sync.Mutex
		active, maxActive := 0, 0
		allIn := make(chan struct{})
		hook := func(_ context.Context, stage aesFilesHookStage, _ string) error {
			if stage != aesFilesHookBeforeOpen {
				return nil
			}
			mu.Lock()
			active++
			maxActive = max(maxActive, active)
			if active == n {
				close(allIn)
			}
			mu.Unlock()
			select {
			case <-allIn:
			case <-time.After(150 * time.Millisecond):
			}
			mu.Lock()
			active--
			mu.Unlock()
			return nil
		}

		err := AESEncryptFilesInDir(dir, filesTestKey, append(tc.opts, filesTestHook(hook))...)
		require.NoError(t, err, tc.name)
		require.LessOrEqual(t, maxActive, tc.limit, "%s: observed %d concurrently active jobs", tc.name, maxActive)
		require.GreaterOrEqual(t, maxActive, 1, tc.name)
	}
}

// TestAESFilesInDirSecurity_Budgets covers below/at/above cases for the file
// count, per-file and aggregate budgets; rejected runs write no output.
// Regression for issue #65.
func TestAESFilesInDirSecurity_Budgets(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a.txt": "12345", "b.txt": "12345", "c.txt": "12345"}
	cases := []struct {
		name string
		opt  AESEncryptFilesInDirOption
		ok   bool
	}{
		{"files at", WithAESFilesInDirMaxFiles(3), true},
		{"files above", WithAESFilesInDirMaxFiles(2), false},
		{"file bytes at", WithAESFilesInDirMaxFileBytes(5), true},
		{"file bytes above", WithAESFilesInDirMaxFileBytes(4), false},
		{"total at", WithAESFilesInDirMaxTotalBytes(15), true},
		{"total above", WithAESFilesInDirMaxTotalBytes(14), false},
	}
	for _, tc := range cases {
		dir := writeFilesTestDir(t, files)
		err := AESEncryptFilesInDir(dir, filesTestKey, tc.opt)
		if tc.ok {
			require.NoError(t, err, tc.name)
			for name, content := range files {
				ct, err := os.ReadFile(filepath.Join(dir, name+defaultEncryptSuffix))
				require.NoError(t, err, tc.name)
				got, err := AesDecrypt(filesTestKey, ct)
				require.NoError(t, err, tc.name)
				require.Equal(t, content, string(got), tc.name)
			}
			continue
		}
		require.Error(t, err, tc.name)
		requireNoEncOutputs(t, dir)
	}
}

// TestAESFilesInDirSecurity_LargeEntryCount checks that enumeration spans
// several ReadDir batches, counts only matching files, and stops at the limit.
// Regression for issue #65.
func TestAESFilesInDirSecurity_LargeEntryCount(t *testing.T) {
	t.Parallel()

	files := map[string]string{}
	for i := range aesFilesReadDirBatch*2 + 10 {
		files[fmt.Sprintf("n%04d.log", i)] = "x"
	}
	files["keep.toml"] = "toml"
	dir := writeFilesTestDir(t, files)

	err := AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirMaxFiles(aesFilesReadDirBatch))
	require.ErrorContains(t, err, "matching files")
	requireNoEncOutputs(t, dir)

	err = AESEncryptFilesInDir(dir, filesTestKey,
		WithAESFilesInDirMaxFiles(1), WithAESFilesInDirFileExt(".toml"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "keep.toml.enc"))
	require.NoError(t, err)
}

// TestAESFilesInDirSecurity_GrowthRaces grows a source after enumeration (seen
// by the descriptor check) and after the descriptor check (seen by the bounded
// read). Regression for issue #65.
func TestAESFilesInDirSecurity_GrowthRaces(t *testing.T) {
	t.Parallel()

	for _, stage := range []aesFilesHookStage{aesFilesHookBeforeOpen, aesFilesHookBeforeRead} {
		dir := writeFilesTestDir(t, map[string]string{"a.txt": "12345"})
		grow := func(_ context.Context, s aesFilesHookStage, name string) error {
			if s != stage {
				return nil
			}
			f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			_, err = f.WriteString("678")
			return errors.Join(err, f.Close())
		}
		err := AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirMaxFileBytes(6), filesTestHook(grow))
		require.ErrorContains(t, err, "per-file budget", "stage %d", stage)
		requireNoEncOutputs(t, dir)
	}

	// Aggregate growth: the second file no longer fits after the first grew.
	dir := writeFilesTestDir(t, map[string]string{"a.txt": "12345", "b.txt": "12345"})
	grow := func(_ context.Context, s aesFilesHookStage, name string) error {
		if s != aesFilesHookBeforeRead || name != "a.txt" {
			return nil
		}
		return os.WriteFile(filepath.Join(dir, name), []byte("123456"), 0o600)
	}
	err := AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirMaxConcurrency(1),
		WithAESFilesInDirMaxTotalBytes(10), filesTestHook(grow))
	require.ErrorContains(t, err, "aggregate budget")
	requireNoStagingFiles(t, dir)
}

// TestAESFilesInDirSecurity_CancellationAndFirstError checks that a canceled
// context does no work and that the first failure stops scheduling.
// Regression for issue #65.
func TestAESFilesInDirSecurity_CancellationAndFirstError(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a.txt": "1", "b.txt": "2", "c.txt": "3", "d.txt": "4"}

	dir := writeFilesTestDir(t, files)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := AESEncryptFilesInDirWithContext(ctx, dir, filesTestKey)
	require.ErrorIs(t, err, context.Canceled)
	requireNoEncOutputs(t, dir)

	errBoom := errors.New("boom")
	var calls atomic.Int32
	dir = writeFilesTestDir(t, files)
	fail := func(_ context.Context, s aesFilesHookStage, _ string) error {
		if s != aesFilesHookBeforeOpen {
			return nil
		}
		calls.Add(1)
		return errBoom
	}
	err = AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirMaxConcurrency(1), filesTestHook(fail))
	require.ErrorIs(t, err, errBoom)
	require.Equal(t, int32(1), calls.Load(), "scheduling must stop after the first error")
	requireNoEncOutputs(t, dir)

	// Cancellation during the run: the first job cancels, later jobs never start.
	calls.Store(0)
	dir = writeFilesTestDir(t, files)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	cancelFirst := func(_ context.Context, s aesFilesHookStage, _ string) error {
		if s == aesFilesHookBeforeRead && calls.Add(1) == 1 {
			cancel()
		}
		return nil
	}
	err = AESEncryptFilesInDirWithContext(ctx, dir, filesTestKey,
		WithAESFilesInDirMaxConcurrency(1), filesTestHook(cancelFirst))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int32(1), calls.Load())
	requireNoEncOutputs(t, dir)
}

// TestAESFilesInDirSecurity_SkipsAndOutputLinks checks skipped entries and that
// existing or dangling output links are never written through. Regression for
// issues #65 and #46.
func TestAESFilesInDirSecurity_SkipsAndOutputLinks(t *testing.T) {
	t.Parallel()

	dir := writeFilesTestDir(t, map[string]string{"a.txt": "plain", "old.txt.enc": "already encrypted"})
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("KEEP"), 0o600))
	require.NoError(t, os.Symlink(victim, filepath.Join(dir, "link.txt")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.txt"), 0o700))

	require.NoError(t, AESEncryptFilesInDir(dir, filesTestKey))
	for _, skipped := range []string{"old.txt.enc.enc", "link.txt.enc", "sub.txt.enc"} {
		_, err := os.Lstat(filepath.Join(dir, skipped))
		require.ErrorIs(t, err, os.ErrNotExist, skipped)
	}
	ct, err := os.ReadFile(filepath.Join(dir, "a.txt.enc"))
	require.NoError(t, err)
	got, err := AEADDecrypt(filesTestKey, ct, nil)
	require.NoError(t, err, "existing AES-GCM format is preserved")
	require.Equal(t, "plain", string(got))

	// An existing regular output is atomically replaced.
	require.NoError(t, AESEncryptFilesInDir(dir, filesTestKey))
	ct2, err := os.ReadFile(filepath.Join(dir, "a.txt.enc"))
	require.NoError(t, err)
	require.NotEqual(t, ct, ct2)

	for name, target := range map[string]string{
		"existing": victim,
		"dangling": filepath.Join(outside, "created-through-link"),
	} {
		d := writeFilesTestDir(t, map[string]string{"a.txt": "plain"})
		link := filepath.Join(d, "a.txt.enc")
		require.NoError(t, os.Symlink(target, link))
		err := AESEncryptFilesInDir(d, filesTestKey)
		require.ErrorContains(t, err, "not a regular file", name)
		dest, err := os.Readlink(link)
		require.NoError(t, err, name)
		require.Equal(t, target, dest, name)
		requireNoStagingFiles(t, d)
	}
	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	require.Equal(t, "KEEP", string(content))
	_, err = os.Lstat(filepath.Join(outside, "created-through-link"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestAESFilesInDirSecurity_PasswordMode checks that a password run derives once
// (shared salt), uses fresh nonces, and accepts empty files. Regression for
// issues #54 and #65.
func TestAESFilesInDirSecurity_PasswordMode(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a.txt": "alpha", "b.txt": "beta", "empty.txt": ""}
	dir := writeFilesTestDir(t, files)
	enc, err := NewPasswordEncryptor([]byte(passwordTestPassword), fastPasswordKDF())
	require.NoError(t, err)
	require.NoError(t, PasswordEncryptFilesInDirWithContext(context.Background(), dir, enc))

	var salts, nonces [][]byte
	for name, content := range files {
		ct, err := os.ReadFile(filepath.Join(dir, name+defaultEncryptSuffix))
		require.NoError(t, err)
		got, err := DecryptByPassword([]byte(passwordTestPassword), ct)
		require.NoError(t, err)
		require.Equal(t, content, string(got))
		salts = append(salts, passwordSalt(ct))
		nonces = append(nonces, passwordNonce(ct))
	}
	require.Equal(t, salts[0], salts[1])
	require.Equal(t, salts[1], salts[2])
	require.NotEqual(t, nonces[0], nonces[1])
	require.NotEqual(t, nonces[1], nonces[2])

	require.Error(t, PasswordEncryptFilesInDirWithContext(context.Background(), dir, nil))
}

// TestAESFilesInDirSecurity_InvalidOptions checks option and key validation.
// Regression for issue #65.
func TestAESFilesInDirSecurity_InvalidOptions(t *testing.T) {
	t.Parallel()

	dir := writeFilesTestDir(t, map[string]string{"a.txt": "x"})
	for name, opt := range map[string]AESEncryptFilesInDirOption{
		"concurrency zero":  WithAESFilesInDirMaxConcurrency(0),
		"concurrency large": WithAESFilesInDirMaxConcurrency(maxAESFilesInDirConcurrency + 1),
		"files zero":        WithAESFilesInDirMaxFiles(0),
		"file bytes zero":   WithAESFilesInDirMaxFileBytes(0),
		"total zero":        WithAESFilesInDirMaxTotalBytes(0),
		"suffix separator":  WithAESFilesInDirFileSuffix("./../x"),
	} {
		require.Error(t, AESEncryptFilesInDir(dir, filesTestKey, opt), name)
	}
	require.Error(t, AESEncryptFilesInDir(dir, []byte("short")), "invalid key length")
	//nolint:staticcheck // a nil context must be rejected rather than panic.
	require.Error(t, AESEncryptFilesInDirWithContext(nil, dir, filesTestKey))
	requireNoEncOutputs(t, dir)
}
