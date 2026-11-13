//go:build linux

package memory

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/transactionlocal"
)

// turnFaultBackend injects mutation and post-commit acknowledgement failures.
type turnFaultBackend struct {
	storageengine.TransactionalEngine
	failAt     int
	afterApply bool
	loseAck    bool
	writes     int
}

// Transaction wraps only the isolated view, leaving real commit behavior intact.
func (backend *turnFaultBackend) Transaction(ctx context.Context, project, session string, fn func(storageengine.Engine) error) error {
	err := backend.TransactionalEngine.Transaction(ctx, project, session, func(view storageengine.Engine) error {
		return fn(&turnFaultView{Engine: view, backend: backend})
	})
	if err == nil && backend.loseAck {
		backend.loseAck = false
		return errors.New("injected lost commit acknowledgement")
	}
	return err
}

// turnFaultView intercepts each write in the callback's private generation.
type turnFaultView struct {
	storageengine.Engine
	backend *turnFaultBackend
}

// Write fails immediately before or after the selected persistent mutation.
func (view *turnFaultView) Write(ctx context.Context, project, path, body string, mode storageengine.WriteMode, offset int64) error {
	view.backend.writes++
	fail := view.backend.writes == view.backend.failAt
	if fail && !view.backend.afterApply {
		return errors.New("injected turn write failure")
	}
	if err := view.Engine.Write(ctx, project, path, body, mode, offset); err != nil {
		return err
	}
	if fail {
		return errors.New("injected turn write failure")
	}
	return nil
}

// privateTransactionRoot creates a test-owned root with the transaction protocol's
// private-write permissions, independently of the test runner's inherited umask.
func privateTransactionRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	return root
}

// newTurnBackend creates a durable backend and an already committed scaffold.
func newTurnBackend(t *testing.T) *transactionlocal.Engine {
	t.Helper()
	backend, err := transactionlocal.NewEngine(local.Config{RootDir: privateTransactionRoot(t)})
	require.NoError(t, err)
	require.NoError(t, backend.Transaction(context.Background(), recoveryProject, recoverySession, func(view storageengine.Engine) error {
		seed, err := NewEngine(view, recoveryConfig())
		if err != nil {
			return err
		}
		return seed.ensureSessionScaffold(context.Background(), recoveryProject, recoverySession)
	}))
	return backend
}

// turnSnapshot reads every persisted file without repair or fallback.
func turnSnapshot(t *testing.T, backend storageengine.Engine) map[string]string {
	t.Helper()
	rows, more, err := backend.List(context.Background(), recoveryProject, "/", 32, 5000)
	require.NoError(t, err)
	require.False(t, more)
	files := map[string]string{}
	for _, row := range rows {
		if row.Type != storageengine.FileTypeFile {
			continue
		}
		body, err := backend.Read(context.Background(), recoveryProject, row.Path, 0, -1)
		require.NoError(t, err)
		files[row.Path] = body
	}
	return files
}

// assertTurnState checks every legacy and canonical persisted view independently.
func assertTurnState(t *testing.T, backend storageengine.Engine, turns []string) {
	t.Helper()
	reader, err := NewEngine(backend, recoveryConfig())
	require.NoError(t, err)
	require.Empty(t, recoveryStateProblems(t, reader, turns))
}

// TestTransactionalTurnEveryWriteFailure requires complete rollback and exact
// recovery after every write, including ignored metrics/watermark failures.
func TestTransactionalTurnEveryWriteFailure(t *testing.T) {
	baseline := &turnFaultBackend{TransactionalEngine: newTurnBackend(t)}
	engine, err := NewTransactionalEngine(baseline, recoveryConfig())
	require.NoError(t, err)
	require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput("one")))
	require.Positive(t, baseline.writes)
	for _, after := range []bool{false, true} {
		for at := 1; at <= baseline.writes; at++ {
			t.Run(fmt.Sprintf("write_%d_after_%t", at, after), func(t *testing.T) {
				backend := newTurnBackend(t)
				before := turnSnapshot(t, backend)
				fault := &turnFaultBackend{TransactionalEngine: backend, failAt: at, afterApply: after}
				engine, err := NewTransactionalEngine(fault, recoveryConfig())
				require.NoError(t, err)
				require.ErrorContains(t, engine.AfterTurn(context.Background(), recoveryInput("one")), "injected turn write failure")
				require.GreaterOrEqual(t, fault.writes, at, "fault must actually fire")
				require.Equal(t, before, turnSnapshot(t, backend), "failed turn must publish nothing")
				// A fresh engine must recover, without any process-local recovery state.
				retry, err := NewTransactionalEngine(backend, recoveryConfig())
				require.NoError(t, err)
				require.NoError(t, retry.AfterTurn(context.Background(), recoveryInput("one")))
				assertTurnState(t, backend, []string{"one"})
			})
		}
	}
}

// TestTransactionalTurnCommitAcknowledgementLost proves retries observe the
// marker committed atomically with all artifacts after an ambiguous result.
func TestTransactionalTurnCommitAcknowledgementLost(t *testing.T) {
	backend := newTurnBackend(t)
	faulty := &turnFaultBackend{TransactionalEngine: backend, loseAck: true}
	first, err := NewTransactionalEngine(faulty, recoveryConfig())
	require.NoError(t, err)
	require.ErrorContains(t, first.AfterTurn(context.Background(), recoveryInput("one")), "lost commit acknowledgement")
	assertTurnState(t, backend, []string{"one"})
	second, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	require.NoError(t, second.AfterTurn(context.Background(), recoveryInput("one")))
	assertTurnState(t, backend, []string{"one"})
}

// TestTransactionalTurnConcurrentInstances requires independent backend handles
// and engines to preserve both distinct turns and suppress identical turns.
func TestTransactionalTurnConcurrentInstances(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_%t", same), func(t *testing.T) {
			root := privateTransactionRoot(t)
			firstBackend, err := transactionlocal.NewEngine(local.Config{RootDir: root})
			require.NoError(t, err)
			secondBackend, err := transactionlocal.NewEngine(local.Config{RootDir: root})
			require.NoError(t, err)
			first, err := NewTransactionalEngine(firstBackend, recoveryConfig())
			require.NoError(t, err)
			second, err := NewTransactionalEngine(secondBackend, recoveryConfig())
			require.NoError(t, err)
			turns := []string{"one", "two"}
			if same {
				turns[1] = "one"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() { <-start; results <- first.AfterTurn(ctx, recoveryInput(turns[0])) }()
			go func() { <-start; results <- second.AfterTurn(ctx, recoveryInput(turns[1])) }()
			close(start)
			require.NoError(t, <-results)
			require.NoError(t, <-results)
			if same {
				turns = turns[:1]
			}
			assertTurnState(t, firstBackend, turns)
		})
	}
}

// TestTransactionalTurnRecordCompatibility compares complete old/new writer
// output and exercises the legacy reader against transactionally committed data.
func TestTransactionalTurnRecordCompatibility(t *testing.T) {
	backend := newTurnBackend(t)
	tx, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	legacyBackend := newMemoryStorageMock()
	legacy, err := NewEngine(legacyBackend, recoveryConfig())
	require.NoError(t, err)
	for _, turn := range []string{"one", "two"} {
		require.NoError(t, tx.AfterTurn(context.Background(), recoveryInput(turn)))
		require.NoError(t, legacy.AfterTurn(context.Background(), recoveryInput(turn)))
	}
	require.Equal(t, turnSnapshot(t, legacyBackend), turnSnapshot(t, backend))
	assertTurnState(t, backend, []string{"one", "two"})
	// An old writer is compatible when it participates in the backend transaction.
	require.NoError(t, backend.Transaction(context.Background(), recoveryProject, recoverySession, func(view storageengine.Engine) error {
		writer, err := NewEngine(view, recoveryConfig())
		if err != nil {
			return err
		}
		return writer.AfterTurn(context.Background(), recoveryInput("three"))
	}))
	require.NoError(t, tx.AfterTurn(context.Background(), recoveryInput("four")))
	assertTurnState(t, backend, []string{"one", "two", "three", "four"})
}

// blockingTurnHeuristic blocks one turn while the backend transaction is held.
type blockingTurnHeuristic struct {
	once             sync.Once
	entered, release chan struct{}
}

// ExtractAndMergeFacts waits at a deterministic barrier before producing facts.
func (h *blockingTurnHeuristic) ExtractAndMergeFacts(ctx context.Context, in HeuristicFactInput) (HeuristicFactResult, error) {
	h.once.Do(func() {
		close(h.entered)
		select {
		case <-h.release:
		case <-ctx.Done():
		}
	})
	return recoveryHeuristic{}.ExtractAndMergeFacts(ctx, in)
}

// TestTransactionalTurnCompactionAndMaintenance serializes compaction and
// management calls with a concurrently persisting turn, preserving its tail.
func TestTransactionalTurnCompactionAndMaintenance(t *testing.T) {
	backend := newTurnBackend(t)
	conf := recoveryConfig()
	conf.RecentContextItems = 1
	seed, err := NewTransactionalEngine(backend, conf)
	require.NoError(t, err)
	for _, turn := range []string{"one", "two", "three"} {
		require.NoError(t, seed.AfterTurn(context.Background(), recoveryInput(turn)))
	}
	blocker := &blockingTurnHeuristic{entered: make(chan struct{}), release: make(chan struct{})}
	conf.HeuristicClient = blocker
	writer, err := NewTransactionalEngine(backend, conf)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	written := make(chan error, 1)
	go func() { written <- writer.AfterTurn(ctx, recoveryInput("four")) }()
	select {
	case <-blocker.entered:
	case earlyErr := <-written:
		require.FailNow(t, "turn completed before reaching the heuristic", "early result: %v", earlyErr)
	case <-ctx.Done():
		require.FailNow(t, "turn did not reach the heuristic barrier", ctx.Err().Error())
	}
	prepared := make(chan error, 1)
	go func() {
		_, err := seed.BeforeTurn(ctx, BeforeTurnInput{Project: recoveryProject, SessionID: recoverySession, TurnID: "next", CurrentInput: recoveryInput("next").InputItems, MaxInputTok: 1})
		prepared <- err
	}()
	maintained := make(chan error, 1)
	go func() { maintained <- seed.RunMaintenance(ctx, recoveryProject, recoverySession) }()
	close(blocker.release)
	require.NoError(t, <-written)
	require.NoError(t, <-prepared)
	require.NoError(t, <-maintained)
	reader, err := NewEngine(backend, recoveryConfig())
	require.NoError(t, err)
	raw, err := reader.loadRawEvents(ctx, recoveryProject, recoverySession)
	require.NoError(t, err)
	require.Len(t, raw, 4)
	current, err := reader.loadContextEventsWithFallback(ctx, recoveryProject, recoverySession)
	require.NoError(t, err)
	require.Equal(t, "four-in-0", current[len(current)-1].ID)
	meta, err := reader.loadMeta(ctx, recoveryProject, recoverySession)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"one", "two", "three", "four"}, meta.ProcessedTurnIDs)
}

// TestTransactionalEngineRequiresCapability leaves legacy backends explicitly
// unsupported and rejects canceled transactions without publishing state.
func TestTransactionalEngineRequiresCapability(t *testing.T) {
	_, err := NewTransactionalEngine(newMemoryStorageMock(), recoveryConfig())
	require.ErrorContains(t, err, "does not support")
	backend := newTurnBackend(t)
	engine, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	before := turnSnapshot(t, backend)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, engine.AfterTurn(ctx, recoveryInput("one")), context.Canceled)
	require.Equal(t, before, turnSnapshot(t, backend))
}
