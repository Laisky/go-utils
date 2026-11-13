//go:build linux

package transactionlocal

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// metadataReceipt locates the one sealed retirement receipt in a disposable root.
func metadataReceipt(t *testing.T, root string) (string, []byte) {
	t.Helper()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	var name string
	var body []byte
	for _, entry := range entries {
		if !temporaryName(entry.Name(), "pointer-") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, entry.Name()))
		require.NoError(t, err)
		if !bytes.HasPrefix(content, []byte(retirementHeader)) {
			continue
		}
		require.Empty(t, name, "fixture must expose exactly one typed receipt")
		name = filepath.Join(root, entry.Name())
		body = content
	}
	require.NotEmpty(t, name)
	return name, body
}

// metadataState leaves a durable new snapshot and an owned historical predecessor
// by failing only retirement, after the commit has been acknowledged durable.
func metadataState(t *testing.T) (root, previous, next, receipt string, body []byte) {
	t.Helper()
	root, previous = retirementFixture(t, "generation-custom-metadata")
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	engine.retireSnapshot = func(string) error { return errors.New("injected metadata fixture retirement failure") }
	err = engine.Write(context.Background(), "p", "/value", "new", storage.WriteModeTruncate, 0)
	require.ErrorContains(t, err, "injected metadata fixture retirement failure")
	outcome, published := storage.PublishedCommit(err)
	require.True(t, published)
	require.Equal(t, storage.CommitOutcomeDurable, outcome)
	next, err = engine.current()
	require.NoError(t, err)
	receipt, body = metadataReceipt(t, root)
	return
}

// metadataCurrent changes only a disposable pointer to model a crash-window state.
func metadataCurrent(t *testing.T, root, generation string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "current"), []byte(filepath.Base(generation)), 0o600))
	require.NoError(t, syncDirectory(root))
}

// assertMetadataValue verifies reopenability and the unchanged local reader's
// persisted bytes without requiring that old readers understand control receipts.
func assertMetadataValue(t *testing.T, root, value string) {
	t.Helper()
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	actual, err := engine.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, value, actual)
	current, err := engine.current()
	require.NoError(t, err)
	oldReader, err := local.NewEngine(local.Config{RootDir: current})
	require.NoError(t, err)
	defer func() { require.NoError(t, oldReader.Close()) }()
	actual, err = oldReader.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, value, actual)
	actual, err = oldReader.Read(context.Background(), "p", "/readonly/fixture", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "historical readonly bytes", actual)
}

// TestRetirementUnsealedMetadata gives no ownership to partial/pre-fsync records,
// while an intact old current pointer remains readable after an interrupted write.
func TestRetirementUnsealedMetadata(t *testing.T) {
	for _, content := range []string{retirementFamily, retirementHeader + `{"previous":"generation-custom-metadata"`, retirementHeader + `{}`} {
		t.Run(strings.ReplaceAll(content, "\n", "_"), func(t *testing.T) {
			root, previous, next, receipt, _ := metadataState(t)
			require.NoError(t, os.WriteFile(receipt, []byte(content), 0o600))
			metadataCurrent(t, root, previous)
			assertMetadataValue(t, root, "old")
			saved, err := os.ReadFile(receipt)
			require.NoError(t, err)
			require.Equal(t, content, string(saved), "unsealed metadata must be preserved")
			metadataCurrent(t, root, next)
			_, err = NewEngine(local.Config{RootDir: root})
			require.Error(t, err, "an incomplete receipt cannot adopt an otherwise unrelated custom predecessor")
			old, err := os.ReadFile(filepath.Join(previous, "p/value"))
			require.NoError(t, err)
			require.Equal(t, "old", string(old))
		})
	}
}

// TestRetirementMalformedMetadataNeverAdopts rejects ownership claims whose
// sealed payload, names, identity or size do not satisfy the supported contract.
func TestRetirementMalformedMetadataNeverAdopts(t *testing.T) {
	for _, kind := range []string{"json", "unknown_field", "traversal", "same_names", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			root, previous, _, receipt, original := metadataState(t)
			payload := original[len(retirementHeader) : len(original)-len(retirementSeal)]
			var record retirementRecord
			require.NoError(t, json.Unmarshal(payload, &record))
			switch kind {
			case "json":
				payload = []byte(`{"previous":`)
			case "unknown_field":
				payload = append(append([]byte(nil), payload[:len(payload)-1]...), []byte(`,"unknown":"field"}`)...)
			case "traversal":
				record.Previous = []byte("generation-../outside")
				var err error
				payload, err = json.Marshal(record)
				require.NoError(t, err)
			case "same_names":
				record.Next = record.Previous
				var err error
				payload, err = json.Marshal(record)
				require.NoError(t, err)
			case "oversized":
				payload = []byte(strings.Repeat("x", maxRetirementBytes))
			}
			malformed := retirementHeader + string(payload) + retirementSeal
			require.NoError(t, os.WriteFile(receipt, []byte(malformed), 0o600))
			_, err := NewEngine(local.Config{RootDir: root})
			require.Error(t, err)
			actual, err := os.ReadFile(receipt)
			require.NoError(t, err)
			require.Equal(t, malformed, string(actual))
			old, err := os.ReadFile(filepath.Join(previous, "p/value"))
			require.NoError(t, err)
			require.Equal(t, "old", string(old))
		})
	}
}

// TestRetirementUnsupportedVersion fails closed for a complete future protocol,
// even after the old predecessor is absent and cannot itself explain rejection.
func TestRetirementUnsupportedVersion(t *testing.T) {
	root, previous, _, receipt, body := metadataState(t)
	require.NoError(t, removeOwnedSnapshot(previous))
	unsupported := strings.Replace(string(body), retirementHeader, retirementFamily+"999\n", 1)
	require.NoError(t, os.WriteFile(receipt, []byte(unsupported), 0o600))
	_, err := NewEngine(local.Config{RootDir: root})
	require.ErrorContains(t, err, "unsupported")
	actual, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, unsupported, string(actual))
}

// TestRetirementIdentitySubstitutionNeverAdopts binds ownership to the old inode
// rather than granting permission to a different directory reusing its name.
func TestRetirementIdentitySubstitutionNeverAdopts(t *testing.T) {
	root, previous, _, _, _ := metadataState(t)
	backup := filepath.Join(privateTestRoot(t), "old")
	require.NoError(t, os.Rename(previous, backup))
	t.Cleanup(func() { require.NoError(t, restoreRetirementFixture(backup)) })
	require.NoError(t, os.Mkdir(previous, 0o700))
	marker := filepath.Join(previous, "preserved")
	require.NoError(t, os.WriteFile(marker, []byte("unrelated replacement"), 0o600))
	oldInfo, err := os.Stat(backup)
	require.NoError(t, err)
	replacement, err := os.Stat(previous)
	require.NoError(t, err)
	require.False(t, os.SameFile(oldInfo, replacement), "negative control must use a genuinely different inode")
	_, err = NewEngine(local.Config{RootDir: root})
	require.Error(t, err)
	actual, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "unrelated replacement", string(actual))
}

// TestRetirementMissingReferences accepts legitimate pre/post-publication crash
// windows without requiring that either receipt reference is the current pointer.
func TestRetirementMissingReferences(t *testing.T) {
	for _, kind := range []string{"previous", "next", "both"} {
		t.Run(kind, func(t *testing.T) {
			root, previous, next, _, _ := metadataState(t)
			value := "new"
			switch kind {
			case "previous":
				require.NoError(t, removeOwnedSnapshot(previous))
			case "next":
				metadataCurrent(t, root, previous)
				require.NoError(t, removeOwnedSnapshot(next))
				value = "old"
			case "both":
				require.NoError(t, removeOwnedSnapshot(previous))
				later := filepath.Join(root, "generation-987654321987654321")
				require.NoError(t, os.Rename(next, later))
				metadataCurrent(t, root, later)
			}
			assertMetadataValue(t, root, value)
		})
	}
}

// TestRetirementLegacyPointerPreserved retains historical untyped temporaries
// without interpreting their bytes as authorization to adopt custom siblings.
func TestRetirementLegacyPointerPreserved(t *testing.T) {
	root, _, next, _, _ := metadataState(t)
	legacy := filepath.Join(root, "pointer-987654321987654321")
	body := []byte(filepath.Base(next))
	require.NoError(t, os.WriteFile(legacy, body, 0o600))
	assertMetadataValue(t, root, "new")
	actual, err := os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, body, actual)
	// The receipt budget does not reinterpret or truncate large untyped
	// historical files that older readers treated as preserved temporaries.
	large := bytes.Repeat([]byte("legacy raw pointer fixture"), maxRetirementBytes)
	require.NoError(t, os.WriteFile(legacy, large, 0o600))
	assertMetadataValue(t, root, "new")
	actual, err = os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, large, actual)
	unrelated := filepath.Join(root, "generation-legacy-archive")
	require.NoError(t, os.Mkdir(unrelated, 0o700))
	marker := filepath.Join(unrelated, "preserved")
	require.NoError(t, os.WriteFile(marker, []byte("legacy pointer is not ownership"), 0o600))
	require.NoError(t, os.WriteFile(legacy, []byte(filepath.Base(unrelated)), 0o600))
	_, err = NewEngine(local.Config{RootDir: root})
	require.Error(t, err)
	actual, err = os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "legacy pointer is not ownership", string(actual))
}

// TestRetirementMissingHistoricalProof keeps pre-upgrade failed-retirement roots
// fail-closed while their selected project's legacy bytes remain exportable.
func TestRetirementMissingHistoricalProof(t *testing.T) {
	root, previous, next, receipt, _ := metadataState(t)
	require.NoError(t, os.Remove(receipt))
	_, err := NewEngine(local.Config{RootDir: root})
	require.Error(t, err, "a pre-upgrade orphan without durable ownership cannot be inferred safe")
	require.DirExists(t, previous)
	oldReader, err := local.NewEngine(local.Config{RootDir: next})
	require.NoError(t, err)
	defer func() { require.NoError(t, oldReader.Close()) }()
	value, err := oldReader.Read(context.Background(), "p", "/value", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "new", value, "explicit quiescent export can recover unchanged project bytes")
}

// TestRetirementDurabilityBarrierOrdering requires predecessor deletion to be
// durable before receipt removal, and accepts receipt resurrection after a
// failed final directory barrier without replaying the committed mutation.
func TestRetirementDurabilityBarrierOrdering(t *testing.T) {
	for _, failurePhase := range []int{1, 2} {
		t.Run(string(rune('0'+failurePhase)), func(t *testing.T) {
			root, previous := retirementFixture(t, "generation-custom-phase")
			engine, err := NewEngine(local.Config{RootDir: root})
			require.NoError(t, err)
			injected := errors.New("injected retirement barrier failure")
			calls := 0
			var receipt string
			var body []byte
			engine.retirementSync = func(path string) error {
				calls++
				require.NoFileExists(t, filepath.Join(previous, "p/value"))
				require.NoDirExists(t, previous, "the predecessor must be removed before either retirement barrier")
				if calls == 1 {
					receipt, body = metadataReceipt(t, root)
				} else {
					require.NoFileExists(t, receipt, "receipt removal must precede only the final barrier")
				}
				if calls == failurePhase {
					return injected
				}
				return syncDirectory(path)
			}
			err = engine.Write(context.Background(), "p", "/value", "new", storage.WriteModeTruncate, 0)
			require.ErrorIs(t, err, injected)
			require.Equal(t, failurePhase, calls)
			outcome, published := storage.PublishedCommit(err)
			require.True(t, published)
			require.Equal(t, storage.CommitOutcomeDurable, outcome)
			if failurePhase == 1 {
				require.FileExists(t, receipt)
			} else {
				require.NoFileExists(t, receipt)
			}
			assertMetadataValue(t, root, "new")
			if failurePhase == 2 {
				// Model either legal post-crash result of the failed receipt-unlink sync:
				// absence above, or resurrection of its prior durable bytes here.
				require.NoError(t, os.WriteFile(receipt, body, 0o600))
				require.NoError(t, syncDirectory(root))
				assertMetadataValue(t, root, "new")
			}
		})
	}
}

// TestRetirementByteExactNames preserves previously supported non-UTF8 current
// pointer names in receipts rather than JSON replacing their original bytes.
func TestRetirementByteExactNames(t *testing.T) {
	active := "generation-byte-\xff"
	root, previous := retirementFixture(t, active)
	engine, err := NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	engine.retireSnapshot = func(string) error { return errors.New("injected byte-name retirement failure") }
	err = engine.Write(context.Background(), "p", "/value", "new", storage.WriteModeTruncate, 0)
	require.ErrorContains(t, err, "injected byte-name retirement failure")
	_, body := metadataReceipt(t, root)
	var record retirementRecord
	require.NoError(t, json.Unmarshal(body[len(retirementHeader):len(body)-len(retirementSeal)], &record))
	require.Equal(t, []byte(active), record.Previous)
	require.DirExists(t, previous)
	assertMetadataValue(t, root, "new")
}
