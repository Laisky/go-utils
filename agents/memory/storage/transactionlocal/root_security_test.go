//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestTransactionRootWritePermissions rejects an existing unsafe root without
// silently changing permissions; read/traverse permissions remain caller policy.
func TestTransactionRootWritePermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0o770, 0o777} {
		t.Run(mode.String(), func(t *testing.T) {
			root := privateTestRoot(t)
			require.NoError(t, os.Chmod(root, mode))
			_, err := NewEngine(local.Config{RootDir: root})
			require.ErrorContains(t, err, "writable")
			info, err := os.Stat(root)
			require.NoError(t, err)
			require.Equal(t, mode, info.Mode().Perm(), "constructor must not silently chmod caller's directory")
		})
	}
	for _, mode := range []os.FileMode{0o700, 0o755} {
		t.Run(mode.String(), func(t *testing.T) {
			root := privateTestRoot(t)
			require.NoError(t, os.Chmod(root, mode))
			engine, err := NewEngine(local.Config{RootDir: root, FilePerm: 0o640, DirPerm: 0o750})
			require.NoError(t, err)
			require.NoError(t, engine.Write(context.Background(), "p", "/value", "fixture", storage.WriteModeTruncate, 0))
		})
	}
}

// TestTransactionRootAncestorPermissions rejects a replaceable root entry and
// accepts a trusted sticky temporary-directory ancestor protecting its owner.
func TestTransactionRootAncestorPermissions(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		name := "replaceable"
		mode := os.FileMode(0o777)
		if sticky {
			name = "sticky"
			mode |= os.ModeSticky
		}
		t.Run(name, func(t *testing.T) {
			parent := privateTestRoot(t)
			root := filepath.Join(parent, "root")
			require.NoError(t, os.Mkdir(root, 0o700))
			require.NoError(t, os.Chmod(parent, mode))
			_, err := NewEngine(local.Config{RootDir: root})
			if sticky {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "ancestor")
			}
		})
	}
}

// TestTransactionRootOwnership rejects a root owned by a different identity
// before enumerating or creating transaction-control entries in that directory.
func TestTransactionRootOwnership(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can access other users' namespaces; non-root ownership control required")
	}
	_, err := NewEngine(local.Config{RootDir: "/"})
	require.ErrorContains(t, err, "owner")
}

// TestTransactionRootRuntimeValidation refuses a root made writable or replaced
// after construction, before opening a lock in the untrusted replacement.
func TestTransactionRootRuntimeValidation(t *testing.T) {
	t.Run("permissions", func(t *testing.T) {
		engine := newTestEngine(t)
		require.NoError(t, os.Chmod(engine.root, 0o777))
		_, err := engine.Read(context.Background(), "p", "/value", 0, -1)
		require.ErrorContains(t, err, "writable")
	})
	t.Run("identity", func(t *testing.T) {
		engine := newTestEngine(t)
		require.NoError(t, os.Rename(engine.root, engine.root+"-moved"))
		t.Cleanup(func() { require.NoError(t, os.RemoveAll(engine.root+"-moved")) })
		require.NoError(t, os.Mkdir(engine.root, 0o700))
		_, err := engine.Read(context.Background(), "p", "/value", 0, -1)
		require.ErrorContains(t, err, "identity")
		_, err = os.Stat(filepath.Join(engine.root, "lock"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

// TestTransactionRootSymlinkPolicy resolves a safe alias once while rejecting
// an alias to an unsafe directory; later alias changes cannot redirect the engine.
func TestTransactionRootSymlinkPolicy(t *testing.T) {
	parent := privateTestRoot(t)
	root := filepath.Join(parent, "actual")
	require.NoError(t, os.Mkdir(root, 0o700))
	alias := filepath.Join(parent, "alias")
	require.NoError(t, os.Symlink(root, alias))
	engine, err := NewEngine(local.Config{RootDir: alias})
	require.NoError(t, err)
	other := filepath.Join(parent, "other")
	require.NoError(t, os.Mkdir(other, 0o700))
	require.NoError(t, os.Remove(alias))
	require.NoError(t, os.Symlink(other, alias))
	require.NoError(t, engine.Write(context.Background(), "p", "/value", "original-root", storage.WriteModeTruncate, 0))
	_, err = os.Stat(filepath.Join(other, "current"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.Chmod(other, 0o777))
	_, err = NewEngine(local.Config{RootDir: alias})
	require.ErrorContains(t, err, "writable")
}

// TestTransactionLockTrust rejects a substituted control inode before the
// transaction callback runs, retaining normal cross-instance lock behavior.
func TestTransactionLockTrust(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "writable"} {
		t.Run(kind, func(t *testing.T) {
			engine := newTestEngine(t)
			lock := filepath.Join(engine.root, "lock")
			target := filepath.Join(privateTestRoot(t), "target")
			require.NoError(t, os.WriteFile(target, []byte("unchanged"), 0o600))
			switch kind {
			case "symlink":
				require.NoError(t, os.Symlink(target, lock))
			case "hardlink":
				require.NoError(t, os.Link(target, lock))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(lock, 0o600))
			case "writable":
				require.NoError(t, os.WriteFile(lock, nil, 0o600))
				require.NoError(t, os.Chmod(lock, 0o666))
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			called := false
			err := engine.Transaction(ctx, "p", "s", func(storage.Engine) error { called = true; return nil })
			require.Error(t, err)
			require.False(t, called)
			require.NotErrorIs(t, err, context.DeadlineExceeded, "invalid special lock must be rejected, not block until deadline")
			body, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, "unchanged", string(body))
		})
	}
}

// TestTransactionPublishedControlTrust rejects unsafe imported/replaced pointer
// files and generation directories, even when their parent root is protected.
func TestTransactionPublishedControlTrust(t *testing.T) {
	for _, kind := range []string{"pointer_writable", "pointer_symlink", "pointer_hardlink", "pointer_fifo", "generation_writable"} {
		t.Run(kind, func(t *testing.T) {
			engine := newTestEngine(t)
			require.NoError(t, engine.Write(context.Background(), "p", "/value", "fixture", storage.WriteModeTruncate, 0))
			pointer := filepath.Join(engine.root, "current")
			generation, err := os.ReadFile(pointer)
			require.NoError(t, err)
			alternate := filepath.Join(engine.root, "pointer-fixture")
			switch kind {
			case "pointer_writable":
				require.NoError(t, os.Chmod(pointer, 0o666))
			case "pointer_symlink":
				require.NoError(t, os.Rename(pointer, alternate))
				require.NoError(t, os.Symlink("pointer-fixture", pointer))
			case "pointer_hardlink":
				require.NoError(t, os.Link(pointer, alternate))
			case "pointer_fifo":
				require.NoError(t, os.Remove(pointer))
				require.NoError(t, syscall.Mkfifo(pointer, 0o600))
			case "generation_writable":
				require.NoError(t, os.Chmod(filepath.Join(engine.root, string(generation)), 0o777))
			}

			_, err = engine.Read(context.Background(), "p", "/value", 0, -1)
			require.Error(t, err)
		})
	}
}
