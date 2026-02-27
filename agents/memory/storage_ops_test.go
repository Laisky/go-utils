package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// TestAppendAndLoadJSONLHelpers verifies append/load helpers and fallback loading behavior.
func TestAppendAndLoadJSONLHelpers(t *testing.T) {
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.appendJSONL(context.Background(), "demo", "/memory/x/test.jsonl", []LogEvent{{ID: "1", TS: now.Format(time.RFC3339), Type: "input_item"}})
	require.NoError(t, err)
	err = engine.appendJSONL(context.Background(), "demo", "/memory/x/test.jsonl", nil)
	require.NoError(t, err)

	lines, err := engine.loadJSONL(context.Background(), "demo", "/memory/x/test.jsonl")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	lines, err = engine.loadJSONL(context.Background(), "demo", "/memory/x/not-found.jsonl")
	require.NoError(t, err)
	require.Nil(t, lines)

	err = storage.Write(context.Background(), "demo", "/memory/x/bad.jsonl", "{bad}\n{}\n", files.WriteModeTruncate, 0)
	require.NoError(t, err)

	events, err := engine.loadContextEvents(context.Background(), "demo", "/memory/x/bad.jsonl")
	require.NoError(t, err)
	require.Len(t, events, 1)

	facts, err := engine.loadFactsFromFile(context.Background(), "demo", "/memory/x/bad.jsonl")
	require.NoError(t, err)
	require.Len(t, facts, 1)

	err = storage.Write(context.Background(), "demo", legacyContextPath("fallback"), "{\"id\":\"x\",\"ts\":\"2026-02-15T00:00:00Z\",\"type\":\"input_item\"}\n", files.WriteModeTruncate, 0)
	require.NoError(t, err)
	fallbackEvents, err := engine.loadContextEventsWithFallback(context.Background(), "demo", "fallback")
	require.NoError(t, err)
	require.Len(t, fallbackEvents, 1)
}

// TestLoadRecallFactsFallback verifies recall fallback to legacy facts and recall limiting.
func TestLoadRecallFactsFallback(t *testing.T) {
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }, RecallFactsLimit: 1})
	require.NoError(t, err)

	legacyFactBody, err := marshalFacts([]MemoryFact{
		{ID: "f1", TS: now.Format(time.RFC3339), Type: "fact_upsert", FactID: "a", Key: "a", Value: "1", Confidence: 0.8},
		{ID: "f2", TS: now.Add(-time.Hour).Format(time.RFC3339), Type: "fact_upsert", FactID: "b", Key: "b", Value: "2", Confidence: 0.7},
	})
	require.NoError(t, err)

	err = storage.Write(context.Background(), "demo", legacyFactsPath("s-fallback"), legacyFactBody, files.WriteModeTruncate, 0)
	require.NoError(t, err)

	facts, err := engine.loadRecallFacts(context.Background(), "demo", "s-fallback", "")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "a", facts[0].FactID)
}

// TestEnsureSessionScaffoldAndMetaIO verifies scaffold idempotency and metadata read/write paths.
func TestEnsureSessionScaffoldAndMetaIO(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.ensureSessionScaffold(context.Background(), "demo", "scaffold")
	require.NoError(t, err)
	// Run again to cover already-exists branches.
	err = engine.ensureSessionScaffold(context.Background(), "demo", "scaffold")
	require.NoError(t, err)

	meta := MemoryMeta{Version: 1, LatestTurnID: "t1", ProcessedTurnIDs: []string{"t1"}, UpdatedAt: now.Format(time.RFC3339)}
	err = engine.writeMeta(context.Background(), "demo", "scaffold", meta)
	require.NoError(t, err)

	loaded, err := engine.loadMeta(context.Background(), "demo", "scaffold")
	require.NoError(t, err)
	require.Equal(t, "t1", loaded.LatestTurnID)

	// Empty file should still return default version.
	err = storage.Write(context.Background(), "demo", "/memory/scaffold/meta/empty.json", "", files.WriteModeTruncate, 0)
	require.NoError(t, err)
	emptyMeta, err := engine.loadMetaFile(context.Background(), "demo", "/memory/scaffold/meta/empty.json")
	require.NoError(t, err)
	require.Equal(t, 1, emptyMeta.Version)
}

// TestListFilesAndTierLoads verifies list filtering and tier loading across shards.
func TestListFilesAndTierLoads(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	facts := []MemoryFact{{ID: "f1", TS: now.Format(time.RFC3339), Type: "fact_upsert", FactID: "id-1", Key: "k", Value: "v", Tier: memoryTierL2}}
	body, err := marshalFacts(facts)
	require.NoError(t, err)

	l2Path := tierFactsShardPath("list", memoryTierL2, now)
	err = storage.Write(context.Background(), "demo", l2Path, body, files.WriteModeTruncate, 0)
	require.NoError(t, err)
	err = storage.Write(context.Background(), "demo", "/memory/list/memory_tiers/L2/readme.txt", "x", files.WriteModeTruncate, 0)
	require.NoError(t, err)

	filesOnly, err := engine.listFiles(context.Background(), "demo", tierRootPath("list", memoryTierL2), ".jsonl")
	require.NoError(t, err)
	require.Len(t, filesOnly, 1)

	tierFacts, err := engine.loadTierFacts(context.Background(), "demo", "list", memoryTierL2)
	require.NoError(t, err)
	require.Len(t, tierFacts, 1)
	require.Equal(t, "id-1", tierFacts[0].FactID)
}
