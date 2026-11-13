//go:build linux

package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/transactionlocal"
)

// poisonTransactionCleanup creates an obsolete snapshot that cannot be removed
// by an unprivileged process, forcing a real error after durable publication.
func poisonTransactionCleanup(t *testing.T, root string) func() {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("cleanup permission fault requires an unprivileged process")
	}
	blocked := filepath.Join(root, "generation-blocked")
	require.NoError(t, os.Mkdir(blocked, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "child"), []byte("synthetic"), 0o600))
	require.NoError(t, os.Chmod(blocked, 0))
	restore := func() { require.NoError(t, os.Chmod(blocked, 0o700)) }
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	return restore
}

// TestTransactionalBeforeTurnPostCommitResult requires a known committed recall
// to retain its prepared output rather than forcing another metrics increment.
func TestTransactionalBeforeTurnPostCommitResult(t *testing.T) {
	root := privateTransactionRoot(t)
	backend, err := transactionlocal.NewEngine(local.Config{RootDir: root})
	require.NoError(t, err)
	engine, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	require.NoError(t, engine.AfterTurn(t.Context(), recoveryInput("one")))
	restore := poisonTransactionCleanup(t, root)
	in := BeforeTurnInput{Project: recoveryProject, SessionID: recoverySession, TurnID: "next", CurrentInput: recoveryInput("one").InputItems}
	out, err := engine.BeforeTurn(t.Context(), in)
	require.ErrorContains(t, err, "remove obsolete generation")
	outcome, published := storageengine.PublishedCommit(err)
	require.True(t, published)
	require.Equal(t, storageengine.CommitOutcomeDurable, outcome)
	reader, readErr := NewEngine(backend, recoveryConfig())
	require.NoError(t, readErr)
	first, readErr := reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
	require.NoError(t, readErr)
	require.Positive(t, first.RecallCount, "the recall metrics must actually be committed")
	require.NotEmpty(t, out.InputItems, "known committed BeforeTurn must preserve its prepared result with the diagnostic")
	restore()
	// Avoid retrying an acknowledged committed operation; cleanup can be attempted
	// independently without replaying recall. This does not make arbitrary retries safe.
	require.NoError(t, backend.Transaction(context.Background(), recoveryProject, recoverySession, func(_ storageengine.Engine) error { return nil }))
}

// outcomeBackend simulates a backend that can classify a lost acknowledgement
// after a complete commit, without rerunning the callback internally.
type outcomeBackend struct {
	storageengine.TransactionalEngine
	outcome storageengine.CommitOutcome
	calls   int
}

// Transaction publishes once and then returns a classified acknowledgement loss.
func (backend *outcomeBackend) Transaction(ctx context.Context, project, session string, fn func(storageengine.Engine) error) error {
	backend.calls++
	if err := backend.TransactionalEngine.Transaction(ctx, project, session, fn); err != nil {
		return err
	}
	return storageengine.NewCommitError(errors.New("synthetic lost acknowledgement"), backend.outcome)
}

// TestTransactionalPublishedOutputs distinguishes result availability from replay
// safety. Each explicit BeforeTurn invocation still counts a real recall; the
// wrapper never silently repeats an operation after either commit outcome.
func TestTransactionalPublishedOutputs(t *testing.T) {
	for _, outcome := range []storageengine.CommitOutcome{storageengine.CommitOutcomeDurable, storageengine.CommitOutcomeUncertain} {
		t.Run(outcomeName(outcome), func(t *testing.T) {
			backend := newTurnBackend(t)
			seed, err := NewTransactionalEngine(backend, recoveryConfig())
			require.NoError(t, err)
			require.NoError(t, seed.AfterTurn(t.Context(), recoveryInput("one")))
			lost := &outcomeBackend{TransactionalEngine: backend, outcome: outcome}
			engine, err := NewTransactionalEngine(lost, recoveryConfig())
			require.NoError(t, err)
			in := BeforeTurnInput{Project: recoveryProject, SessionID: recoverySession, TurnID: "next", CurrentInput: recoveryInput("one").InputItems}
			out, err := engine.BeforeTurn(t.Context(), in)
			actual, published := storageengine.PublishedCommit(err)
			require.True(t, published)
			require.Equal(t, outcome, actual)
			require.NotEmpty(t, out.InputItems)
			require.NotEmpty(t, out.RecallFactIDs)
			require.Equal(t, 1, lost.calls)
			reader, err := NewEngine(backend, recoveryConfig())
			require.NoError(t, err)
			first, err := reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Positive(t, first.RecallCount)
			// A deliberate repeated invocation is a second recall, not an invisible
			// recovery retry. No incompatible durable operation-ID policy is introduced.
			_, err = engine.BeforeTurn(t.Context(), in)
			require.Error(t, err)
			next, err := reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Equal(t, 2*first.RecallCount, next.RecallCount)
			require.Equal(t, 2, lost.calls)
			summaries, err := engine.ListDirWithAbstract(t.Context(), recoveryProject, recoverySession, "", 1, 20)
			require.Error(t, err)
			require.NotEmpty(t, summaries, "published summary results remain available")
		})
	}
}

// outcomeName returns a descriptive name for each tested published outcome.
func outcomeName(outcome storageengine.CommitOutcome) string {
	if outcome == storageengine.CommitOutcomeDurable {
		return "durable"
	}
	return "uncertain"
}
