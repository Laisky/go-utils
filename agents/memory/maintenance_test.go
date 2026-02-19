package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// TestRunMaintenanceArchivesAndSweeps verifies archive and expiration sweep behavior.
func TestRunMaintenanceArchivesAndSweeps(t *testing.T) {
	now := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		TimeNow:          func() time.Time { return now },
		CompactionMinAge: 24 * time.Hour,
	})
	require.NoError(t, err)

	oldShardDate := now.AddDate(0, 0, -2)
	err = mockStorage.Write(context.Background(), "demo", rawLogShardPath("m1", oldShardDate), "{\"id\":\"old\"}\n", files.WriteModeTruncate, 0)
	require.NoError(t, err)

	l1Path := tierFactsShardPath("m1", memoryTierL1, now)
	activeFact := MemoryFact{
		ID:        "f-active",
		TS:        now.Format(time.RFC3339),
		Type:      "fact_upsert",
		FactID:    "active",
		Key:       "task",
		Value:     "keep",
		Tier:      memoryTierL1,
		ExpiresAt: now.AddDate(0, 0, 1).Format(time.RFC3339),
	}
	expiredFact := MemoryFact{
		ID:        "f-expired",
		TS:        now.AddDate(0, 0, -3).Format(time.RFC3339),
		Type:      "fact_upsert",
		FactID:    "expired",
		Key:       "task",
		Value:     "remove",
		Tier:      memoryTierL1,
		ExpiresAt: now.AddDate(0, 0, -1).Format(time.RFC3339),
	}
	body, err := marshalFacts([]MemoryFact{activeFact, expiredFact})
	require.NoError(t, err)
	err = mockStorage.Write(context.Background(), "demo", l1Path, body, files.WriteModeTruncate, 0)
	require.NoError(t, err)

	err = engine.RunMaintenance(context.Background(), "demo", "m1")
	require.NoError(t, err)

	rawInfo, err := mockStorage.Stat(context.Background(), "demo", rawLogShardPath("m1", oldShardDate))
	require.NoError(t, err)
	require.False(t, rawInfo.Exists)

	archivePath := archiveShardPath("m1", oldShardDate, pathBase(rawLogShardPath("m1", oldShardDate)))
	archiveInfo, err := mockStorage.Stat(context.Background(), "demo", archivePath)
	require.NoError(t, err)
	require.True(t, archiveInfo.Exists)

	updatedL1Body, err := mockStorage.Read(context.Background(), "demo", l1Path, 0, -1)
	require.NoError(t, err)
	require.Contains(t, updatedL1Body, "f-active")
	require.NotContains(t, updatedL1Body, "f-expired")

	rootAbstract, err := mockStorage.Read(context.Background(), "demo", sessionBasePath("m1")+"/.abstract", 0, -1)
	require.NoError(t, err)
	require.GreaterOrEqual(t, wordCount(rootAbstract), 100)
}

// TestListDirWithAbstract verifies list_dir enrichment with .abstract and overview presence.
func TestListDirWithAbstract(t *testing.T) {
	now := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "m2",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "My name is Carol",
			}},
		}},
	})
	require.NoError(t, err)

	summaries, err := engine.ListDirWithAbstract(context.Background(), "demo", "m2", "", 8, 200)
	require.NoError(t, err)
	require.NotEmpty(t, summaries)

	foundRoot := false
	foundTierRoot := false
	for _, summary := range summaries {
		if summary.Path == sessionBasePath("m2") {
			foundRoot = true
			require.NotEmpty(t, strings.TrimSpace(summary.Abstract))
			require.True(t, summary.HasOverview)
		}
		if summary.Path == sessionBasePath("m2")+"/memory_tiers" {
			foundTierRoot = true
		}
	}
	require.True(t, foundRoot)
	require.True(t, foundTierRoot)
}

// TestRunMaintenanceTriggersCompaction verifies maintenance-triggered compaction for oversized runtime context.
func TestRunMaintenanceTriggersCompaction(t *testing.T) {
	now := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		TimeNow:            func() time.Time { return now },
		RecentContextItems: 1,
	})
	require.NoError(t, err)

	for idx := 0; idx < 4; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "m3",
			TurnID:    "t" + string(rune('a'+idx)),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "context entry",
				}},
			}},
		})
		require.NoError(t, err)
	}

	emptyOldShard := rawLogShardPath("m3", now.AddDate(0, 0, -2))
	err = mockStorage.Write(context.Background(), "demo", emptyOldShard, "   \n", files.WriteModeTruncate, 0)
	require.NoError(t, err)

	err = mockStorage.Write(
		context.Background(),
		"demo",
		sessionBasePath("m3")+"/events/raw/not-a-date/log.jsonl",
		"{\"id\":\"x\"}\n",
		files.WriteModeTruncate,
		0,
	)
	require.NoError(t, err)

	err = engine.RunMaintenance(context.Background(), "demo", "m3")
	require.NoError(t, err)

	emptyInfo, err := mockStorage.Stat(context.Background(), "demo", emptyOldShard)
	require.NoError(t, err)
	require.False(t, emptyInfo.Exists)

	ctxBody, err := mockStorage.Read(context.Background(), "demo", runtimeContextPath("m3"), 0, -1)
	require.NoError(t, err)
	require.Contains(t, ctxBody, "\"type\":\"compact_summary\"")
}

// TestRefreshDirectorySummaryTruncation verifies summary files are truncated to policy limits.
func TestRefreshDirectorySummaryTruncation(t *testing.T) {
	now := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	abstractWords := make([]string, 240)
	for idx := range abstractWords {
		abstractWords[idx] = fmt.Sprintf("segment%d", idx)
	}
	dir := "/memory/m4/" + strings.Join(abstractWords, " ")

	entryWords := make([]string, 140)
	for idx := range entryWords {
		entryWords[idx] = fmt.Sprintf("entry%d", idx)
	}
	longName := strings.Join(entryWords, " ")
	for idx := 0; idx < 25; idx++ {
		filePath := fmt.Sprintf("%s/file-%d-%s.jsonl", dir, idx, longName)
		err = mockStorage.Write(context.Background(), "demo", filePath, "{\"id\":\"x\"}\n", files.WriteModeTruncate, 0)
		require.NoError(t, err)
	}

	err = engine.refreshDirectorySummary(context.Background(), "demo", dir)
	require.NoError(t, err)

	abstractBody, err := mockStorage.Read(context.Background(), "demo", dir+"/.abstract", 0, -1)
	require.NoError(t, err)
	require.LessOrEqual(t, wordCount(abstractBody), 200)

	overviewBody, err := mockStorage.Read(context.Background(), "demo", dir+"/.overview", 0, -1)
	require.NoError(t, err)
	require.LessOrEqual(t, wordCount(overviewBody), 2000)
}

// pathBase returns the final path segment and is used by archive assertions.
func pathBase(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
