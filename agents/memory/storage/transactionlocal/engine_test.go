//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// privateTestRoot creates a test-owned directory with explicit private permissions.
// testing.TempDir's numbered child follows the process umask, so it is not
// necessarily private even though its own generated parent is owner-only.
func privateTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	return root
}

// newTestEngine returns an adapter rooted in a private temporary directory.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(local.Config{RootDir: privateTestRoot(t)})
	require.NoError(t, err)
	return engine
}

// TestTransactionRollbackAndLifetime checks rollback, caught write errors, and expired views.
func TestTransactionRollbackAndLifetime(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/value", "before", storage.WriteModeTruncate, 0))
	var retained storage.Engine
	require.ErrorContains(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		retained = view
		require.NoError(t, view.Write(ctx, "p", "/value", "partial", storage.WriteModeTruncate, 0))
		return errors.New("injected failure")
	}), "injected failure")
	body, err := engine.Read(ctx, "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "before", body)
	require.ErrorContains(t, retained.Write(ctx, "p", "/value", "late", storage.WriteModeAppend, 0), "closed")
	require.Error(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		_ = view.Write(ctx, "other", "/value", "invalid", storage.WriteModeAppend, 0)
		return nil
	}))
	body, err = engine.Read(ctx, "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "before", body)
	require.Error(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		_ = view.Write(ctx, "p", "/value", "invalid", storage.WriteModeTruncate, -1)
		return nil
	}))
	body, err = engine.Read(ctx, "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "before", body)
}

// TestTransactionCancellationAndIsolation checks canceled waiters and precommit cancellation.
func TestTransactionCancellationAndIsolation(t *testing.T) {
	engine := newTestEngine(t)
	other, err := NewEngine(local.Config{RootDir: engine.root})
	require.NoError(t, err)
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- engine.Transaction(context.Background(), "p", "s", func(view storage.Engine) error {
			close(locked)
			<-release
			return view.Write(context.Background(), "p", "/value", "first", storage.WriteModeTruncate, 0)
		})
	}()
	<-locked
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	called := false
	err = other.Transaction(ctx, "p", "other", func(storage.Engine) error { called = true; return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
	close(release)
	require.NoError(t, <-done)
	ctx, cancel = context.WithCancel(context.Background())
	err = other.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		require.NoError(t, view.Write(ctx, "p", "/value", "canceled", storage.WriteModeTruncate, 0))
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	body, err := engine.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "first", body)
}

// TestTransactionProcessHelper runs only in isolated child test processes.
func TestTransactionProcessHelper(t *testing.T) {
	mode := os.Getenv("GO_UTILS_TRANSACTION_TEST_MODE")
	if mode == "" {
		return
	}
	engine, err := NewEngine(local.Config{RootDir: os.Getenv("GO_UTILS_TRANSACTION_TEST_ROOT")})
	require.NoError(t, err)
	err = engine.Transaction(context.Background(), "p", "s", func(view storage.Engine) error {
		body, readErr := view.Read(context.Background(), "p", "/value", 0, -1)
		if readErr != nil {
			return readErr
		}
		value := 0
		if body != "" {
			value, readErr = strconv.Atoi(body)
			if readErr != nil {
				return errors.Wrap(readErr, "parse counter")
			}
		}
		if writeErr := view.Write(context.Background(), "p", "/value", strconv.Itoa(value+1), storage.WriteModeTruncate, 0); writeErr != nil {
			return writeErr
		}
		if mode == "crash" {
			os.Exit(23)
		}
		return nil
	})
	require.NoError(t, err)
}

// helperCommand constructs a child process with only synthetic test configuration.
func helperCommand(t *testing.T, root, mode string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestTransactionProcessHelper$")
	command.Env = append(os.Environ(), "GO_UTILS_TRANSACTION_TEST_ROOT="+root, "GO_UTILS_TRANSACTION_TEST_MODE="+mode)
	return command
}

// TestTransactionProcessesAndCrash checks durable restart and actual interprocess exclusion.
func TestTransactionProcessesAndCrash(t *testing.T) {
	engine := newTestEngine(t)
	require.NoError(t, engine.Write(context.Background(), "p", "/value", "0", storage.WriteModeTruncate, 0))
	crashed := helperCommand(t, engine.root, "crash")
	output, err := crashed.CombinedOutput()
	require.Error(t, err, string(output))
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 23, exit.ExitCode())
	reopened, err := NewEngine(local.Config{RootDir: engine.root})
	require.NoError(t, err)
	body, err := reopened.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "0", body)
	commands := []*exec.Cmd{}
	for range 4 {
		command := helperCommand(t, engine.root, "commit")
		require.NoError(t, command.Start())
		commands = append(commands, command)
	}
	for _, command := range commands {
		require.NoError(t, command.Wait())
	}
	reopened, err = NewEngine(local.Config{RootDir: engine.root})
	require.NoError(t, err)
	body, err = reopened.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "4", body)
	entries, err := os.ReadDir(engine.root)
	require.NoError(t, err)
	require.Len(t, entries, 3, "only current, lock and live generation survive cleanup")
}

// TestTransactionStorageBehavior checks unchanged local content formats and methods.
func TestTransactionStorageBehavior(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		require.NoError(t, view.Write(ctx, "p", "/dir/file", "abcdef", storage.WriteModeTruncate, 0))
		require.NoError(t, view.Write(ctx, "p", "/dir/file", "Z", storage.WriteModeOverwrite, 2))
		return view.Write(ctx, "p", "/dir/file", "!", storage.WriteModeAppend, 0)
	}))
	body, err := engine.Read(ctx, "p", "/dir/file", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "abZdef!", body)
	pointer, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	legacy, err := local.NewEngine(local.Config{RootDir: filepath.Join(engine.root, string(pointer))})
	require.NoError(t, err)
	defer func() { require.NoError(t, legacy.Close()) }()
	body, err = legacy.Read(ctx, "p", "/dir/file", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "abZdef!", body)
	_, err = engine.Stat(ctx, "p", "/dir/file")
	require.NoError(t, err)
	entries, more, err := engine.List(ctx, "p", "/dir", 2, 10)
	require.NoError(t, err)
	require.False(t, more)
	require.NotEmpty(t, entries)
	chunks, err := engine.Search(ctx, "p", "abZdef", "/dir", 10)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
	require.NoError(t, engine.Delete(ctx, "p", "/dir", true))
	body, err = engine.Read(ctx, "p", "/dir/file", 0, -1)
	require.NoError(t, err)
	require.Empty(t, body)
}

// TestTransactionRejectsLegacyRoot prevents accidental use of an existing local destination.
func TestTransactionRejectsLegacyRoot(t *testing.T) {
	root := privateTestRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, "legacy-project"), 0o700))
	_, err := NewEngine(local.Config{RootDir: root})
	require.ErrorContains(t, err, "dedicated directory")
}

// TestTransactionInvalidConfigurationAndPointers verifies immediate validation and fail-closed recovery.
func TestTransactionInvalidConfigurationAndPointers(t *testing.T) {
	_, err := NewEngine(local.Config{RootDir: privateTestRoot(t), Limits: local.Limits{MaxReadBytes: -1}})
	require.ErrorContains(t, err, "negative")
	for _, pointer := range []string{"generation-../../outside", "generation-missing", string(make([]byte, 256))} {
		engine := newTestEngine(t)
		require.NoError(t, os.WriteFile(filepath.Join(engine.root, "current"), []byte(pointer), 0o600))
		_, err = engine.Read(context.Background(), "p", "/value", 0, -1)
		require.Error(t, err)
		called := false
		err = engine.Transaction(context.Background(), "p", "s", func(storage.Engine) error { called = true; return nil })
		require.Error(t, err)
		require.False(t, called)
	}
}
