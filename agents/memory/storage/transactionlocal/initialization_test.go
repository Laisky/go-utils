//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestTransactionExistingRootTraverseOnlyAncestor checks that reopening a
// private existing root never requires read access to unrelated ancestors.
func TestTransactionExistingRootTraverseOnlyAncestor(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the genuine directory-read permission control")
	}
	for _, mode := range []os.FileMode{0o311, 0o711} {
		t.Run(mode.String(), func(t *testing.T) {
			base := privateTestRoot(t)
			ancestor := filepath.Join(base, "ancestor")
			root := filepath.Join(ancestor, "root")
			require.NoError(t, os.MkdirAll(root, 0o700))
			require.NoError(t, os.Chmod(ancestor, mode))
			t.Cleanup(func() { require.NoError(t, os.Chmod(ancestor, 0o700)) })
			// This verifies the OS permission condition instead of assuming that a
			// mode value blocks the current user:0711 still grants its owner read.
			opened, err := os.Open(ancestor)
			if mode == 0o311 {
				require.ErrorIs(t, err, os.ErrPermission)
			} else {
				require.NoError(t, err)
				require.NoError(t, opened.Close())
			}
			engine, err := NewEngine(local.Config{RootDir: root})
			require.NoError(t, err)
			require.NoError(t, engine.Write(context.Background(), "p", "/value", "fixture", storage.WriteModeTruncate, 0))
			body, err := engine.Read(context.Background(), "p", "/value", 0, -1)
			require.NoError(t, err)
			require.Equal(t, "fixture", body)
		})
	}
}

// TestRootInitializationSyncScope verifies successful mkdir ownership tracking
// and the minimal bottom-up directory-entry durability boundary.
func TestRootInitializationSyncScope(t *testing.T) {
	base := privateTestRoot(t)
	first := filepath.Join(base, "first")
	second := filepath.Join(first, "second")
	root := filepath.Join(second, "root")
	created, err := makeRootDirectories(root)
	require.NoError(t, err)
	require.Equal(t, []string{first, second, root}, created)
	calls := []string{}
	require.NoError(t, syncCreatedDirectories(created, func(path string) error {
		calls = append(calls, path)
		return syncDirectory(path)
	}))
	require.Equal(t, []string{root, second, first, base}, calls, "only newly created entries and their immediate parents need synchronization")
	for _, directory := range created {
		info, err := os.Stat(directory)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
	// Reopening an existing root must not issue even one constructor fsync.
	created, err = makeRootDirectories(root)
	require.NoError(t, err)
	require.Empty(t, created)
	calls = nil
	require.NoError(t, syncCreatedDirectories(created, func(path string) error {
		calls = append(calls, path)
		return errors.New("unexpected synchronization for pre-existing root")
	}))
	require.Empty(t, calls)
	// A symlink alias to an existing safe root is not a new directory either.
	alias := filepath.Join(base, "alias")
	require.NoError(t, os.Symlink(root, alias))
	created, err = makeRootDirectories(alias)
	require.NoError(t, err)
	require.Empty(t, created)
}

// TestRootInitializationSyncFailure retains necessary durability errors instead
// of treating reduced synchronization scope as permission-error suppression.
func TestRootInitializationSyncFailure(t *testing.T) {
	base := privateTestRoot(t)
	root := filepath.Join(base, "root")
	created, err := makeRootDirectories(root)
	require.NoError(t, err)
	injected := errors.New("injected directory synchronization failure")
	calls := []string{}
	err = syncCreatedDirectories(created, func(path string) error {
		calls = append(calls, path)
		if path == base {
			return injected
		}
		return syncDirectory(path)
	})
	require.ErrorIs(t, err, injected)
	require.Equal(t, []string{root, base}, calls)
	// A concurrently existing component is never claimed as a new directory.
	created, err = makeRootDirectories(root)
	require.NoError(t, err)
	require.Empty(t, created)
	file := filepath.Join(base, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err = makeRootDirectories(filepath.Join(file, "root"))
	require.Error(t, err)
}

// TestTransactionNewRootRequiresParentDurability distinguishes a genuinely new
// root under a traverse-only ancestor from reopening an existing root there.
func TestTransactionNewRootRequiresParentDurability(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the directory-read permission condition")
	}
	base := privateTestRoot(t)
	parent := filepath.Join(base, "parent")
	require.NoError(t, os.Mkdir(parent, 0o700))
	require.NoError(t, os.Chmod(parent, 0o311))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o700)) })
	root := filepath.Join(parent, "root")
	_, err := NewEngine(local.Config{RootDir: root})
	require.ErrorIs(t, err, os.ErrPermission, "the new root's parent entry still requires a successful fsync")
	_, err = os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist, "failed initialization removes only its newly created empty root")
}

// TestRootInitializationFailureCleanup removes only directories this constructor
// created, retaining existing roots and any concurrent nonempty contents.
func TestRootInitializationFailureCleanup(t *testing.T) {
	base := privateTestRoot(t)
	root := filepath.Join(base, "first", "second", "root")
	_, err := NewEngine(local.Config{RootDir: root, Limits: local.Limits{MaxReadBytes: -1}})
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(base, "first"))
	require.ErrorIs(t, err, os.ErrNotExist)
	existing := filepath.Join(base, "existing")
	require.NoError(t, os.Mkdir(existing, 0o700))
	_, err = NewEngine(local.Config{RootDir: existing, Limits: local.Limits{MaxReadBytes: -1}})
	require.Error(t, err)
	info, err := os.Stat(existing)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	created, err := makeRootDirectories(root)
	require.NoError(t, err)
	marker := filepath.Join(root, "preserved")
	require.NoError(t, os.WriteFile(marker, []byte("concurrent fixture"), 0o600))
	require.Error(t, removeCreatedDirectories(created))
	body, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "concurrent fixture", string(body))
}
