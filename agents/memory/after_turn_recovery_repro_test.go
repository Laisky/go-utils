//go:build memory_integrity_repro

package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// failAppendStorage injects one failure at the runtime append after raw persistence.
type failAppendStorage struct {
	storageengine.Engine
	failed bool
}

// Write fails the first runtime-context append, then forwards subsequent writes.
func (s *failAppendStorage) Write(ctx context.Context, project, p, body string, mode storageengine.WriteMode, offset int64) error {
	if p == runtimeContextPath("recovery") && mode == storageengine.WriteModeAppend && !s.failed {
		s.failed = true
		return errors.New("injected runtime append failure")
	}
	return s.Engine.Write(ctx, project, p, body, mode, offset)
}

// recoveryInput returns a deterministic turn with one event and one unique fact.
func recoveryInput(turn string) AfterTurnInput {
	return AfterTurnInput{Project: "demo", SessionID: "recovery", TurnID: turn,
		InputItems: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "Remember " + turn}}}},
	}
}

// recoveryHeuristic emits distinct facts so concurrent index replacement is observable.
type recoveryHeuristic struct{}

// ExtractAndMergeFacts returns one stable fact keyed by the supplied turn ID.
func (recoveryHeuristic) ExtractAndMergeFacts(_ context.Context, in HeuristicFactInput) (HeuristicFactResult, error) {
	return HeuristicFactResult{UpdatedFacts: []MemoryFact{{ID: in.TurnID, FactID: in.TurnID, Key: in.TurnID, Value: in.TurnID, Tier: memoryTierL0}}}, nil
}

// recoveryConfig returns a fixed clock and deterministic heuristic for reproductions.
func recoveryConfig() Config {
	return Config{TimeNow: func() time.Time { return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) }, HeuristicClient: recoveryHeuristic{}}
}

// TestAfterTurnRecoveryPartialFailure requires retries not to duplicate any log.
// This acceptance test intentionally fails until issue #145 is resolved.
func TestAfterTurnRecoveryPartialFailure(t *testing.T) {
	ctx := context.Background()
	backend := newMemoryStorageMock()
	faulty := &failAppendStorage{Engine: backend}
	engine, err := NewEngine(faulty, recoveryConfig())
	require.NoError(t, err)
	require.NoError(t, engine.ensureSessionScaffold(ctx, "demo", "recovery"))
	in := recoveryInput("one")
	require.ErrorContains(t, engine.AfterTurn(ctx, in), "injected runtime append failure")
	require.NoError(t, engine.AfterTurn(ctx, in))
	for _, p := range []string{rawLogShardPath(in.SessionID, recoveryConfig().TimeNow()), runtimeContextPath(in.SessionID), legacyLogPath(in.SessionID), legacyContextPath(in.SessionID)} {
		t.Run(p, func(t *testing.T) {
			events, err := engine.loadContextEvents(ctx, in.Project, p)
			require.NoError(t, err)
			require.Len(t, events, 1, "retry must persist each event exactly once")
		})
	}
}

// barrierStorage forces two independent engines to consume the same snapshots.
type barrierStorage struct {
	storageengine.Engine
	mu       sync.Mutex
	arrivals map[string]int
	release  map[string]chan struct{}
}

// Read captures the snapshot before waiting for the other reader of that file.
func (s *barrierStorage) Read(ctx context.Context, project, p string, offset, length int64) (string, error) {
	body, err := s.Engine.Read(ctx, project, p, offset, length)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	gate := s.release[p]
	if gate == nil || s.arrivals[p] >= 2 {
		s.mu.Unlock()
		return body, nil
	}
	s.arrivals[p]++
	if s.arrivals[p] == 2 {
		close(gate)
	}
	s.mu.Unlock()
	select {
	case <-gate:
		return body, nil
	case <-ctx.Done():
		return "", errors.Wrap(ctx.Err(), "wait for snapshot barrier")
	}
}

// TestAfterTurnRecoveryConcurrentInstances requires concurrent turns to converge.
// Barriers make the lost update deterministic rather than scheduler dependent.
func TestAfterTurnRecoveryConcurrentInstances(t *testing.T) {
	for _, sameTurn := range []bool{false, true} {
		name := "distinct_turns"
		if sameTurn {
			name = "identical_turn"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			backend := newMemoryStorageMock()
			seed, err := NewEngine(backend, recoveryConfig())
			require.NoError(t, err)
			require.NoError(t, seed.ensureSessionScaffold(ctx, "demo", "recovery"))
			require.NoError(t, seed.writeMeta(ctx, "demo", "recovery", MemoryMeta{Version: 1}))
			require.NoError(t, seed.writeActiveFactsIndex(ctx, "demo", "recovery", ActiveFactsIndex{Facts: map[string]MemoryFact{}}))
			barrier := &barrierStorage{Engine: backend, arrivals: make(map[string]int), release: map[string]chan struct{}{
				metaStatePath("recovery"): make(chan struct{}), activeFactsIndexPath("recovery"): make(chan struct{}),
			}}
			first, err := NewEngine(barrier, recoveryConfig())
			require.NoError(t, err)
			second, err := NewEngine(barrier, recoveryConfig())
			require.NoError(t, err)
			turns := []string{"one", "two"}
			if sameTurn {
				turns[1] = turns[0]
			}
			results := make(chan error, 2)
			go func() { results <- first.AfterTurn(ctx, recoveryInput(turns[0])) }()
			go func() { results <- second.AfterTurn(ctx, recoveryInput(turns[1])) }()
			require.NoError(t, <-results)
			require.NoError(t, <-results)
			meta, err := seed.loadMeta(ctx, "demo", "recovery")
			require.NoError(t, err)
			index, err := seed.loadActiveFactsIndex(ctx, "demo", "recovery")
			require.NoError(t, err)
			events, err := seed.loadContextEvents(ctx, "demo", rawLogShardPath("recovery", recoveryConfig().TimeNow()))
			require.NoError(t, err)
			if sameTurn {
				require.Len(t, events, 1, "same turn must commit once across engines")
			} else {
				t.Run("metadata", func(t *testing.T) { require.ElementsMatch(t, turns, meta.ProcessedTurnIDs) })
				t.Run("facts", func(t *testing.T) { require.Len(t, index.Facts, 2, "both turn facts must survive") })
			}
		})
	}
}
