package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAfterTurnIdempotent verifies duplicated turn writes do not duplicate records.
func TestAfterTurnIdempotent(t *testing.T) {
	now := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	in := AfterTurnInput{
		Project:   "demo",
		SessionID: "s1",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "My name is Alice. I prefer concise answers. Today I need finish report.",
			}},
		}},
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "Noted.",
			}},
		}},
	}

	err = engine.AfterTurn(context.Background(), in)
	require.NoError(t, err)
	err = engine.AfterTurn(context.Background(), in)
	require.NoError(t, err)

	rawBody, err := mockStorage.Read(context.Background(), "demo", rawLogShardPath("s1", now), 0, -1)
	require.NoError(t, err)
	require.Equal(t, 2, len(nonEmptyLines(rawBody)))

	ctxBody, err := mockStorage.Read(context.Background(), "demo", runtimeContextPath("s1"), 0, -1)
	require.NoError(t, err)
	require.Equal(t, 2, len(nonEmptyLines(ctxBody)))

	metaBody, err := mockStorage.Read(context.Background(), "demo", metaStatePath("s1"), 0, -1)
	require.NoError(t, err)
	require.Contains(t, metaBody, "t1")

	l0Body, err := mockStorage.Read(context.Background(), "demo", tierFactsShardPath("s1", memoryTierL0, now), 0, -1)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(nonEmptyLines(l0Body)), 2)

	l1Body, err := mockStorage.Read(context.Background(), "demo", tierFactsShardPath("s1", memoryTierL1, now), 0, -1)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(nonEmptyLines(l1Body)), 1)
}

// TestBeforeTurnRecall verifies facts and history are recalled into input items.
func TestBeforeTurnRecall(t *testing.T) {
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "s2",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "I prefer concise answers and I like golang.",
			}},
		}},
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "Noted",
			}},
		}},
	})
	require.NoError(t, err)

	out, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s2",
		TurnID:       "t2",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "help me"}}}},
		MaxInputTok:  120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.InputItems)
	require.NotEmpty(t, out.RecallFactIDs)
	require.Greater(t, out.ContextTokenCount, 0)

	foundMemoryBlock := false
	for _, item := range out.InputItems {
		if item.Role == "developer" && len(item.Content) > 0 {
			require.Contains(t, item.Content[0].Text, "Memory recall")
			require.Contains(t, item.Content[0].Text, "[L0]")
			foundMemoryBlock = true
			break
		}
	}
	require.True(t, foundMemoryBlock)
}

// TestBeforeTurnCompaction verifies context compaction writes compact records in new layout.
func TestBeforeTurnCompaction(t *testing.T) {
	now := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		RecentContextItems: 2,
		CompactThreshold:   0.8,
		TimeNow:            func() time.Time { return now },
	})
	require.NoError(t, err)

	for idx := 0; idx < 6; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "s3",
			TurnID:    "t" + string(rune('a'+idx)),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "This is a long sentence to force compaction and exceed token budget.",
				}},
			}},
		})
		require.NoError(t, err)
	}

	_, err = engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s3",
		TurnID:       "t-final",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "continue"}}}},
		MaxInputTok:  10,
	})
	require.NoError(t, err)

	ctxBody, err := mockStorage.Read(context.Background(), "demo", runtimeContextPath("s3"), 0, -1)
	require.NoError(t, err)
	require.Contains(t, ctxBody, "\"type\":\"compact_summary\"")

	compactBody, err := mockStorage.Read(context.Background(), "demo", compactShardPath("s3", now), 0, -1)
	require.NoError(t, err)
	require.Contains(t, compactBody, "compact_summary")

	pointerBody, err := mockStorage.Read(context.Background(), "demo", latestCompactPointerPath("s3"), 0, -1)
	require.NoError(t, err)
	require.Contains(t, pointerBody, "last_compact_at")
}

// TestAfterTurnUsesLegacyMetaFallback verifies legacy meta fallback prevents duplicated turn processing.
func TestAfterTurnUsesLegacyMetaFallback(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	legacyMeta := map[string]any{
		"version":        1,
		"processed_turn": []string{"legacy-turn"},
	}
	metaBuf, err := json.Marshal(legacyMeta)
	require.NoError(t, err)
	err = mockStorage.Write(context.Background(), "demo", legacyMetaPath("s4"), string(metaBuf), "TRUNCATE", 0)
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "s4",
		TurnID:    "legacy-turn",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "noop",
			}},
		}},
	})
	require.NoError(t, err)

	rawBody, err := mockStorage.Read(context.Background(), "demo", rawLogShardPath("s4", now), 0, -1)
	require.NoError(t, err)
	require.Empty(t, rawBody)
}

// nonEmptyLines splits text and returns non-empty lines.
func nonEmptyLines(body string) []string {
	return parseJSONLLines(body)
}
