//go:build linux

package memory

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// turnReadFaultBackend injects a read failure into one transactional view.
type turnReadFaultBackend struct {
	storageengine.TransactionalEngine
	path  string
	fired bool
}

// Transaction wraps reads in an isolated generation without changing commit behavior.
func (backend *turnReadFaultBackend) Transaction(ctx context.Context, project, session string, fn func(storageengine.Engine) error) error {
	return backend.TransactionalEngine.Transaction(ctx, project, session, func(view storageengine.Engine) error {
		return fn(&turnReadFaultView{Engine: view, backend: backend})
	})
}

// turnReadFaultView fails the first read of a selected existing metadata file.
type turnReadFaultView struct {
	storageengine.Engine
	backend *turnReadFaultBackend
}

// Read returns the injected error once and delegates all other reads.
func (view *turnReadFaultView) Read(ctx context.Context, project, path string, offset, length int64) (string, error) {
	if path == view.backend.path && !view.backend.fired {
		view.backend.fired = true
		return "", errors.New("injected turn metadata read failure")
	}
	return view.Engine.Read(ctx, project, path, offset, length)
}

// TestTransactionalTurnMetadataReadFailure requires rollback when legacy code
// suppresses a watermark or metrics read error, followed by a complete fresh retry.
func TestTransactionalTurnMetadataReadFailure(t *testing.T) {
	for _, path := range []string{metaWatermarksPath(recoverySession), metaMetricsPath(recoverySession)} {
		t.Run(path, func(t *testing.T) {
			backend := newTurnBackend(t)
			before := turnSnapshot(t, backend)
			faulty := &turnReadFaultBackend{TransactionalEngine: backend, path: path}
			engine, err := NewTransactionalEngine(faulty, recoveryConfig())
			require.NoError(t, err)
			require.ErrorContains(t, engine.AfterTurn(t.Context(), recoveryInput("one")), "injected turn metadata read failure")
			require.True(t, faulty.fired, "the intended read failure must occur")
			require.Equal(t, before, turnSnapshot(t, backend), "no turn artifacts or processed marker may survive the failed metadata read")
			retry, err := NewTransactionalEngine(backend, recoveryConfig())
			require.NoError(t, err)
			require.NoError(t, retry.AfterTurn(t.Context(), recoveryInput("one")))
			assertTurnState(t, backend, []string{"one"})
		})
	}
}

// TestTransactionalTurnMalformedMetadata rejects decoding failures without a
// processed marker, while the unchanged legacy engine retains best-effort behavior.
func TestTransactionalTurnMalformedMetadata(t *testing.T) {
	for _, path := range []string{metaWatermarksPath(recoverySession), metaMetricsPath(recoverySession)} {
		t.Run(path, func(t *testing.T) {
			backend := newTurnBackend(t)
			require.NoError(t, backend.Transaction(t.Context(), recoveryProject, recoverySession, func(view storageengine.Engine) error {
				return view.Write(t.Context(), recoveryProject, path, "{malformed", storageengine.WriteModeTruncate, 0)
			}))
			before := turnSnapshot(t, backend)
			engine, err := NewTransactionalEngine(backend, recoveryConfig())
			require.NoError(t, err)
			require.Error(t, engine.AfterTurn(t.Context(), recoveryInput("one")))
			require.Equal(t, before, turnSnapshot(t, backend), "decoding failure must not publish turn artifacts or a processed marker")
			// Legacy behavior remains explicitly best effort even inside a supplied transaction.
			require.NoError(t, backend.Transaction(t.Context(), recoveryProject, recoverySession, func(view storageengine.Engine) error {
				legacy, err := NewEngine(view, recoveryConfig())
				if err != nil {
					return err
				}
				return legacy.AfterTurn(t.Context(), recoveryInput("one"))
			}))
			reader, err := NewEngine(backend, recoveryConfig())
			require.NoError(t, err)
			meta, err := reader.loadMeta(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Contains(t, meta.ProcessedTurnIDs, "one", "legacy best-effort semantics must remain unchanged")
		})
	}
}
