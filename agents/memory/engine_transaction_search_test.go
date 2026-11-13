//go:build linux

package memory

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

var errBeforeTurnSearchFixture = errors.New("injected before-turn search failure")

// beforeTurnSearchBackend injects the same Search behavior into legacy and transactional views.
type beforeTurnSearchBackend struct {
	storageengine.TransactionalEngine
	fail  bool
	calls int
}

// Transaction wraps the isolated view so Search errors occur inside the real transaction.
func (backend *beforeTurnSearchBackend) Transaction(
	ctx context.Context, project, session string, fn func(storageengine.Engine) error,
) error {
	return backend.TransactionalEngine.Transaction(ctx, project, session, func(view storageengine.Engine) error {
		return fn(&beforeTurnSearchView{Engine: view, backend: backend})
	})
}

// Search applies the fixture to legacy direct storage calls.
func (backend *beforeTurnSearchBackend) Search(
	ctx context.Context, project, query, prefix string, limit int,
) ([]storageengine.FileChunk, error) {
	return backend.search(backend.TransactionalEngine, ctx, project, query, prefix, limit)
}

// search records every attempted search and injects an error or delegates to storage.
func (backend *beforeTurnSearchBackend) search(
	view storageengine.Engine, ctx context.Context, project, query, prefix string, limit int,
) ([]storageengine.FileChunk, error) {
	backend.calls++
	if backend.fail {
		return nil, errors.WithStack(errBeforeTurnSearchFixture)
	}
	return view.Search(ctx, project, query, prefix, limit)
}

// beforeTurnSearchView changes only Search inside an isolated transaction.
type beforeTurnSearchView struct {
	storageengine.Engine
	backend *beforeTurnSearchBackend
}

// Search applies the fixture without changing the remaining storage operations.
func (view *beforeTurnSearchView) Search(
	ctx context.Context, project, query, prefix string, limit int,
) ([]storageengine.FileChunk, error) {
	return view.backend.search(view.Engine, ctx, project, query, prefix, limit)
}

// TestBeforeTurnSearchFailureContract distinguishes legacy best-effort assembly
// from transactional rollback, with healthy controls proving both engines prepare input.
func TestBeforeTurnSearchFailureContract(t *testing.T) {
	for _, transactional := range []bool{false, true} {
		api := "legacy"
		if transactional {
			api = "transactional"
		}
		for _, fail := range []bool{false, true} {
			state := "healthy"
			if fail {
				state = "search_failure"
			}
			t.Run(api+"/"+state, func(t *testing.T) {
				backend := newTurnBackend(t)
				conf := recoveryConfig()
				seed, err := NewTransactionalEngine(backend, conf)
				require.NoError(t, err)
				require.NoError(t, seed.AfterTurn(t.Context(), recoveryInput("one")))
				before := turnSnapshot(t, backend)
				fault := &beforeTurnSearchBackend{TransactionalEngine: backend, fail: fail}
				var engine Engine
				if transactional {
					engine, err = NewTransactionalEngine(fault, conf)
				} else {
					engine, err = NewEngine(fault, conf)
				}
				require.NoError(t, err)
				current := recoveryInput("query").InputItems
				current[0].Content[0].Text = "one"
				out, err := engine.BeforeTurn(t.Context(), BeforeTurnInput{
					Project: recoveryProject, SessionID: recoverySession, TurnID: "query", CurrentInput: current,
				})
				require.Equal(t, 1, fault.calls, "nonempty query must actually reach Search")
				if transactional && fail {
					require.ErrorIs(t, err, errBeforeTurnSearchFixture)
					require.Equal(t, BeforeTurnOutput{}, out, "aborted request must return no prepared input")
					require.Equal(t, before, turnSnapshot(t, backend), "aborted search must not persist recall metrics or other mutations")
				} else {
					require.NoError(t, err)
					require.NotEmpty(t, out.InputItems)
					require.NotEmpty(t, out.RecallFactIDs, "a recalled fact makes the persisted metrics change observable")
					require.Contains(t, out.InputItems, current[0])
					reader, err := NewEngine(backend, conf)
					require.NoError(t, err)
					metrics, err := reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
					require.NoError(t, err)
					require.Equal(t, len(out.RecallFactIDs), metrics.RecallCount)
					require.NotEqual(t, before, turnSnapshot(t, backend), "successful assembly persists recall metrics")
				}
			})
		}
	}
}
