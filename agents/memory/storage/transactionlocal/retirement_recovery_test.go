//go:build linux

package transactionlocal

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

const retirementRecoveryExit = 29

// retirementFixture creates a disposable historical active generation with readonly payloads.
func retirementFixture(t *testing.T, active string) (string, string) {
	t.Helper()
	root := privateTestRoot(t)
	t.Cleanup(func() { require.NoError(t, restoreRetirementFixture(root)) })
	generation := filepath.Join(root, active)
	backend, err := local.NewEngine(local.Config{RootDir: generation})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, backend.Write(ctx, "p", "/value", "old", storage.WriteModeTruncate, 0))
	require.NoError(t, backend.Write(ctx, "p", "/readonly/fixture", "historical readonly bytes", storage.WriteModeTruncate, 0))
	require.NoError(t, backend.Close())
	require.NoError(t, os.Chmod(filepath.Join(generation, "p/readonly/fixture"), 0o400))
	require.NoError(t, os.Chmod(filepath.Join(generation, "p/readonly"), 0o500))
	require.NoError(t, syncTree(ctx, generation))
	pointer, err := os.OpenFile(filepath.Join(root, "current"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	require.NoError(t, err)
	_, err = pointer.WriteString(active)
	require.NoError(t, err)
	require.NoError(t, pointer.Sync())
	require.NoError(t, pointer.Close())
	require.NoError(t, syncDirectory(root))
	return root, generation
}

// restoreRetirementFixture changes only disposable test-owned directory modes for TempDir cleanup.
func restoreRetirementFixture(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.Wrap(walkErr, "walk disposable retirement fixture")
		}
		if entry.IsDir() {
			return errors.Wrap(os.Chmod(path, 0o700), "restore disposable directory")
		}
		return nil
	})
}

// TestRetirementRecoveryProcessHelper exits at the post-publication retirement boundary.
func TestRetirementRecoveryProcessHelper(t *testing.T) {
	root := os.Getenv("GO_UTILS_RETIREMENT_TEST_ROOT")
	if root == "" {
		return
	}
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	engine.retireSnapshot = func(string) error { os.Exit(retirementRecoveryExit); return nil }
	err = engine.Write(context.Background(), "p", "/value", "new", storage.WriteModeTruncate, 0)
	require.NoError(t, err)
	t.Fatal("post-publication retirement crash hook was not reached")
}

// crashDuringRetirement runs a real child process and verifies its exact crash boundary.
func crashDuringRetirement(t *testing.T, root string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestRetirementRecoveryProcessHelper$")
	command.Env = append(os.Environ(), "GO_UTILS_RETIREMENT_TEST_ROOT="+root)
	output, err := command.CombinedOutput()
	require.Error(t, err, string(output))
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, retirementRecoveryExit, exit.ExitCode(), string(output))
}

// assertRetirementReopens checks repeated opens preserve complete committed and historical data.
func assertRetirementReopens(t *testing.T, root, value string) *Engine {
	t.Helper()
	var reopened *Engine
	for range 2 {
		engine, err := NewEngine(local.Config{RootDir: root})
		require.NoError(t, err, "committed retirement state must remain reopenable")
		body, err := engine.Read(context.Background(), "p", "/value", 0, -1)
		require.NoError(t, err)
		require.Equal(t, value, body)
		body, err = engine.Read(context.Background(), "p", "/readonly/fixture", 0, -1)
		require.NoError(t, err)
		require.Equal(t, "historical readonly bytes", body)
		snapshot, err := engine.current()
		require.NoError(t, err)
		mode, err := os.Stat(filepath.Join(snapshot, "p/readonly"))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o500), mode.Mode().Perm())
		reopened = engine
	}
	return reopened
}

// TestRetirementRecoveryAfterProcessExit requires durable retirement ownership across process loss.
func TestRetirementRecoveryAfterProcessExit(t *testing.T) {
	for _, active := range []string{"generation-import", "generation-custom-active", "generation-bytes-\xff", "generation-123456"} {
		t.Run(active, func(t *testing.T) {
			root, prior := retirementFixture(t, active)
			crashDuringRetirement(t, root)
			require.DirExists(t, prior, "the process must exit before retiring the old generation")
			engine := assertRetirementReopens(t, root, "new")
			require.NoError(t, engine.Write(context.Background(), "p", "/value", "later", storage.WriteModeTruncate, 0))
			assertRetirementReopens(t, root, "later")
			require.DirExists(t, prior, "unrelated later commits must not sweep recorded crash leftovers")
		})
	}
}

// TestRetirementRecoveryAfterRealPermissionFailure keeps a durable commit reopenable after actual EACCES.
func TestRetirementRecoveryAfterRealPermissionFailure(t *testing.T) {
	require.NotZero(t, os.Geteuid(), "actual EACCES retirement behavior requires a nonroot test process")
	for _, active := range []string{"generation-import", "generation-custom-active"} {
		t.Run(active, func(t *testing.T) {
			root, prior := retirementFixture(t, active)
			engine, err := NewEngine(local.Config{RootDir: root})
			require.NoError(t, err)
			syncCalls := 0
			engine.commitSync = func(path string) error {
				syncCalls++
				if syncErr := syncDirectory(path); syncErr != nil {
					return syncErr
				}
				return errors.Wrap(os.Chmod(path, 0o500), "make disposable root read-only after publication")
			}
			err = engine.Write(context.Background(), "p", "/value", "new", storage.WriteModeTruncate, 0)
			require.Equal(t, 1, syncCalls)
			require.ErrorIs(t, err, os.ErrPermission, "the retirement syscall must genuinely fail under nonroot permissions")
			outcome, published := storage.PublishedCommit(err)
			require.True(t, published)
			require.Equal(t, storage.CommitOutcomeDurable, outcome)
			require.DirExists(t, prior)
			assertRetirementReopens(t, root, "new")
			require.NoError(t, os.Chmod(root, 0o700))
			reopened := assertRetirementReopens(t, root, "new")
			require.NoError(t, reopened.Write(context.Background(), "p", "/value", "later", storage.WriteModeTruncate, 0))
			assertRetirementReopens(t, root, "later")
			require.DirExists(t, prior, "failed old retirement remains an explicit operator leftover")
		})
	}
}

// TestRetirementRecoveryDoesNotAdoptUnrelatedSibling rejects and preserves arbitrary similarly named data.
func TestRetirementRecoveryDoesNotAdoptUnrelatedSibling(t *testing.T) {
	root, _ := retirementFixture(t, "generation-123456")
	crashDuringRetirement(t, root)
	_, openErr := NewEngine(local.Config{RootDir: root})
	require.NoError(t, openErr, "the numeric crash fixture must reopen before unrelated data is introduced")
	unrelated := filepath.Join(root, "generation-archive")
	require.NoError(t, os.Mkdir(unrelated, 0o700))
	marker := filepath.Join(unrelated, "preserve")
	require.NoError(t, os.WriteFile(marker, []byte("unrelated disposable data"), 0o600))
	_, err := NewEngine(local.Config{RootDir: root})
	require.Error(t, err)
	body, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "unrelated disposable data", string(body))
}
