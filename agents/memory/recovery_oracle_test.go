package memory

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/stretchr/testify/require"
)

const recoveryProject = "demo"
const recoverySession = "recovery"

var recoveryTime = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// recoveryInput returns a deterministic turn with one event and one unique fact.
func recoveryInput(turn string) AfterTurnInput {
	return AfterTurnInput{Project: recoveryProject, SessionID: recoverySession, TurnID: turn, UserID: "fixture-user",
		InputItems: []ResponseItem{{Metadata: map[string]string{"item_id": turn + "-item"}, Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "Remember " + turn}}}},
	}
}

// recoveryHeuristic emits distinct facts so concurrent index replacement is observable.
type recoveryHeuristic struct{}

// ExtractAndMergeFacts returns one stable fact keyed by the supplied turn ID.
func (recoveryHeuristic) ExtractAndMergeFacts(_ context.Context, in HeuristicFactInput) (HeuristicFactResult, error) {
	return HeuristicFactResult{UpdatedFacts: []MemoryFact{{ID: in.TurnID, FactID: in.TurnID, Key: in.TurnID, Value: in.TurnID, Type: memoryFactTypeUpsert, Tier: memoryTierL0}}}, nil
}

// recoveryConfig returns a fixed clock and deterministic heuristic for reproductions.
func recoveryConfig() Config {
	return Config{TimeNow: func() time.Time { return recoveryTime }, HeuristicClient: recoveryHeuristic{}}
}

// recoveryStateProblems compares independently specified records across every
// event/fact view, both metadata copies, the active index and watermarks. It
// reports all mismatches rather than letting the first count failure hide others.
func recoveryStateProblems(t *testing.T, engine *StandardEngine, turns []string) []string {
	t.Helper()
	ctx := context.Background()
	ts := recoveryTime.Format(time.RFC3339)
	events := make([]LogEvent, 0, len(turns))
	facts := make([]MemoryFact, 0, len(turns))
	indexFacts := make(map[string]MemoryFact)
	for _, turn := range turns {
		in := recoveryInput(turn)
		events = append(events, LogEvent{ID: turn + "-in-0", TS: ts, Type: "input_item", TurnID: turn,
			UserID: "fixture-user", ItemID: turn + "-item", Item: in.InputItems[0]})
		fact := MemoryFact{ID: turn, TS: ts, Type: memoryFactTypeUpsert, FactID: turn, Key: turn,
			Value: turn, Tier: memoryTierL0, State: memoryStateActive, SourceTurnID: turn, SourceUserID: "fixture-user"}
		facts = append(facts, fact)
		indexFacts[turn+"::"+turn] = fact
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	var problems []string
	readJSON := func(p string, out any) {
		body, err := recoveryReadFile(ctx, engine, p)
		if err != nil || json.Unmarshal([]byte(body), out) != nil {
			problems = append(problems, p+": invalid persisted JSON")
		}
	}
	check := func(label string, expected, actual any) {
		if !reflect.DeepEqual(expected, actual) {
			problems = append(problems, label)
		}
	}
	for _, p := range []string{rawLogShardPath(recoverySession, recoveryTime), runtimeContextPath(recoverySession), legacyLogPath(recoverySession), legacyContextPath(recoverySession)} {
		actual, err := recoveryReadRecords[LogEvent](ctx, engine, p)
		if err != nil {
			problems = append(problems, p+": invalid persisted records")
			continue
		}
		sort.Slice(actual, func(i, j int) bool { return actual[i].ID < actual[j].ID })
		check(p, events, actual)
	}
	for _, p := range []string{tierFactsShardPath(recoverySession, memoryTierL0, recoveryTime), legacyFactsPath(recoverySession)} {
		actual, err := recoveryReadRecords[MemoryFact](ctx, engine, p)
		if err != nil {
			problems = append(problems, p+": invalid persisted records")
			continue
		}
		sort.Slice(actual, func(i, j int) bool { return actual[i].ID < actual[j].ID })
		check(p, facts, actual)
	}
	var index ActiveFactsIndex
	readJSON(activeFactsIndexPath(recoverySession), &index)
	check("active index", ActiveFactsIndex{UpdatedAt: ts, Facts: indexFacts}, index)
	var meta MemoryMeta
	readJSON(metaStatePath(recoverySession), &meta)
	processed := append([]string(nil), meta.ProcessedTurnIDs...)
	sort.Strings(processed)
	expectedTurns := append([]string(nil), turns...)
	sort.Strings(expectedTurns)
	check("processed IDs", expectedTurns, processed)
	if len(meta.ProcessedTurnIDs) == 0 {
		problems = append(problems, "missing latest turn")
	} else {
		check("latest turn ordering", meta.ProcessedTurnIDs[len(meta.ProcessedTurnIDs)-1], meta.LatestTurnID)
	}
	check("canonical metadata", MemoryMeta{Version: 1, LatestTurnID: meta.LatestTurnID, ProcessedTurnIDs: meta.ProcessedTurnIDs, UpdatedAt: ts}, meta)
	var legacy MemoryMeta
	readJSON(legacyMetaPath(recoverySession), &legacy)
	check("legacy metadata", meta, legacy)
	var watermarks MemoryWatermarks
	readJSON(metaWatermarksPath(recoverySession), &watermarks)
	check("watermarks", MemoryWatermarks{UpdatedAt: ts, LastRawEventID: meta.LatestTurnID + "-in-0", LastRawEventTS: ts,
		LastProcessedTurnID: meta.LatestTurnID, RawEventCount: len(turns), RuntimeContextCount: len(turns), ActiveFactCount: len(turns)}, watermarks)
	var metrics MemoryMetrics
	readJSON(metaMetricsPath(recoverySession), &metrics)
	check("metrics", MemoryMetrics{UpdatedAt: ts}, metrics)
	return problems
}

// TestRecoveryOracleControls proves the oracle accepts valid sequential state
// and rejects a same-sized fact map with wrong values, which count-only tests miss.
func TestRecoveryOracleControls(t *testing.T) {
	backend := newMemoryStorageMock()
	engine, err := NewEngine(backend, recoveryConfig())
	require.NoError(t, err)
	for _, turn := range []string{"one", "two"} {
		require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput(turn)))
	}
	require.Empty(t, recoveryStateProblems(t, engine, []string{"one", "two"}))
	index, err := engine.loadActiveFactsIndex(context.Background(), recoveryProject, recoverySession)
	require.NoError(t, err)
	for key, fact := range index.Facts {
		fact.Value = "incorrect"
		index.Facts[key] = fact
	}
	require.NoError(t, engine.writeActiveFactsIndex(context.Background(), recoveryProject, recoverySession, index))
	require.Contains(t, recoveryStateProblems(t, engine, []string{"one", "two"}), "active index")
}

// recoveryReadFile reads a required artifact directly, without engine repair,
// fallback, normalization or malformed-record skipping.
func recoveryReadFile(ctx context.Context, engine *StandardEngine, p string) (string, error) {
	info, err := engine.storage.Stat(ctx, recoveryProject, p)
	if err != nil {
		return "", errors.Wrap(err, "stat recovery artifact")
	}
	if !info.Exists {
		return "", errors.Errorf("missing recovery artifact %s", p)
	}
	body, err := engine.storage.Read(ctx, recoveryProject, p, 0, -1)
	if err != nil {
		return "", errors.Wrap(err, "read recovery artifact")
	}
	return body, nil
}

// recoveryReadRecords strictly decodes every nonempty JSONL record, returning
// an error rather than hiding malformed tails as the production readers can.
func recoveryReadRecords[T any](ctx context.Context, engine *StandardEngine, p string) ([]T, error) {
	body, err := recoveryReadFile(ctx, engine, p)
	if err != nil {
		return nil, err
	}
	var records []T
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, errors.Wrap(err, "decode recovery record")
		}
		records = append(records, record)
	}
	return records, nil
}

// TestRecoveryOracleRejectsMalformedArtifacts ensures production recovery
// readers cannot hide malformed JSONL or missing/corrupt authoritative indexes.
func TestRecoveryOracleRejectsMalformedArtifacts(t *testing.T) {
	for _, kind := range []string{"jsonl_tail", "missing_index", "corrupt_index", "metadata_version"} {
		t.Run(kind, func(t *testing.T) {
			backend := newMemoryStorageMock()
			engine, err := NewEngine(backend, recoveryConfig())
			require.NoError(t, err)
			ctx := context.Background()
			require.NoError(t, engine.AfterTurn(ctx, recoveryInput("one")))
			require.Empty(t, recoveryStateProblems(t, engine, []string{"one"}))
			switch kind {
			case "jsonl_tail":
				require.NoError(t, backend.Write(ctx, recoveryProject, legacyLogPath(recoverySession), "{invalid}\n", "APPEND", 0))
			case "missing_index":
				require.NoError(t, backend.Delete(ctx, recoveryProject, activeFactsIndexPath(recoverySession), false))
			case "corrupt_index":
				require.NoError(t, backend.Write(ctx, recoveryProject, activeFactsIndexPath(recoverySession), "{invalid}", "TRUNCATE", 0))
			case "metadata_version":
				body, err := recoveryReadFile(ctx, engine, metaStatePath(recoverySession))
				require.NoError(t, err)
				body = strings.Replace(body, `"version":1`, `"version":0`, 1)
				require.NoError(t, backend.Write(ctx, recoveryProject, metaStatePath(recoverySession), body, "TRUNCATE", 0))
			}
			require.NotEmpty(t, recoveryStateProblems(t, engine, []string{"one"}), "strict oracle must reject corrupted persistent bytes")
		})
	}
}
