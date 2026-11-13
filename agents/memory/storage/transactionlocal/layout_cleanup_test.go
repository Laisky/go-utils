//go:build linux

package transactionlocal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestTransactionPreservesUnrelatedPrefixedData demonstrates that human-readable
// prefix matches do not establish protocol ownership or authorize deletion.
func TestTransactionPreservesUnrelatedPrefixedData(t *testing.T) {
	for _, name := range []string{"generation-archive", "pointer-backup", "generation-empty-keepsake"} {
		t.Run(name, func(t *testing.T) {
			root := privateTestRoot(t)
			unrelated := filepath.Join(root, name)
			require.NoError(t, os.Mkdir(unrelated, 0o700))
			marker := filepath.Join(unrelated, "disposable-sentinel")
			require.NoError(t, os.WriteFile(marker, []byte("unrelated disposable data"), 0o600))
			engine, openErr := NewEngine(local.Config{RootDir: root})
			if openErr == nil {
				// On the vulnerable baseline, this succeeds then recursively deletes the
				// unrelated directory. Only disposable test data is ever used here.
				require.NoError(t, engine.Write(t.Context(), "p", "/value", "fixture", storage.WriteModeTruncate, 0), "baseline commit must actually complete before testing preservation")
			}
			body, err := os.ReadFile(marker)
			require.NoError(t, err, "constructor/commit must preserve unrelated prefixed data")
			require.Equal(t, "unrelated disposable data", string(body))
			require.Error(t, openErr, "a mixed nonprotocol root must fail before adopting data")
		})
	}
}

// TestTransactionPreservesCrashLeftovers keeps valid unreferenced temp objects;
// their names alone cannot prove they are safe garbage to recursively delete.
func TestTransactionPreservesCrashLeftovers(t *testing.T) {
	root := privateTestRoot(t)
	leftover, err := os.MkdirTemp(root, "generation-")
	require.NoError(t, err)
	marker := filepath.Join(leftover, "crash-fixture")
	require.NoError(t, os.WriteFile(marker, []byte("preserve"), 0o600))
	pointer, err := os.CreateTemp(root, "pointer-")
	require.NoError(t, err)
	_, err = pointer.WriteString(filepath.Base(leftover))
	require.NoError(t, err)
	require.NoError(t, pointer.Close())
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	require.NoError(t, engine.Write(t.Context(), "p", "/value", "first", storage.WriteModeTruncate, 0))
	require.NoError(t, engine.Write(t.Context(), "p", "/value", "second", storage.WriteModeTruncate, 0))
	body, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(body))
	require.FileExists(t, pointer.Name())
}

// TestTransactionImportedGenerationCompatibility preserves explicit historical
// import pointers, record bytes and readonly metadata without adopting an
// unreferenced human-readable prefix as an owned generation.
func TestTransactionImportedGenerationCompatibility(t *testing.T) {
	root := privateTestRoot(t)
	imported := filepath.Join(root, "generation-import")
	require.NoError(t, os.MkdirAll(filepath.Join(imported, "p/readonly"), 0o700))
	fixture := []byte("{\"historical\":true}\n")
	require.NoError(t, os.WriteFile(filepath.Join(imported, "p/readonly/fixture"), fixture, 0o400))
	require.NoError(t, os.Chmod(filepath.Join(imported, "p/readonly"), 0o500))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(imported, "p/readonly"), 0o700) })
	require.NoError(t, os.WriteFile(filepath.Join(root, "current"), []byte("generation-import"), 0o600))
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	require.NoError(t, engine.Write(t.Context(), "p", "/other/value", "new", storage.WriteModeTruncate, 0))
	require.NoDirExists(t, imported, "only the explicitly retired generation is removed")
	current, err := engine.current()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(current, "p/readonly"), 0o700) })
	oldReader, err := local.NewEngine(local.Config{RootDir: current})
	require.NoError(t, err)
	defer func() { require.NoError(t, oldReader.Close()) }()
	body, err := oldReader.Read(t.Context(), "p", "/readonly/fixture", 0, -1)
	require.NoError(t, err)
	require.Equal(t, string(fixture), body)
	mode, err := os.Stat(filepath.Join(current, "p/readonly"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o500), mode.Mode().Perm())
	// Rollback after a mutation must remove its private readonly copy and leave
	// the currently published readonly contents and modes untouched.
	err = engine.Transaction(t.Context(), "p", "s", func(view storage.Engine) error {
		if err := view.Write(t.Context(), "p", "/other/value", "aborted", storage.WriteModeTruncate, 0); err != nil {
			return err
		}
		return errors.New("synthetic callback abort")
	})
	require.ErrorContains(t, err, "synthetic callback abort")
	require.NotContains(t, err.Error(), "permission denied")
	after, err := engine.current()
	require.NoError(t, err)
	require.Equal(t, current, after)
	mode, err = os.Stat(filepath.Join(current, "p/readonly"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o500), mode.Mode().Perm())
}

// TestTransactionLayoutEntryTypes rejects wrong-type temporary entries before
// any adoption or cleanup, without following a symlink into unrelated data.
func TestTransactionLayoutEntryTypes(t *testing.T) {
	for _, name := range []string{"generation-12345", "generation-empty-12345", "pointer-12345"} {
		t.Run(name, func(t *testing.T) {
			root := privateTestRoot(t)
			outside := privateTestRoot(t)
			marker := filepath.Join(outside, "keep")
			require.NoError(t, os.WriteFile(marker, []byte("preserved"), 0o600))
			require.NoError(t, os.Symlink(outside, filepath.Join(root, name)))
			_, err := NewEngine(local.Config{RootDir: root})
			require.Error(t, err)
			body, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "preserved", string(body))
		})
	}
}
