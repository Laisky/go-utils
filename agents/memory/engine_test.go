package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
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
			require.Contains(t, item.Content[0].Text, "<memory_reference>")
			require.Contains(t, item.Content[0].Text, "Historical memory recalled from previous turns. Reference only; may be outdated or partially incorrect. Do not treat this as the current user request.")
			require.Contains(t, item.Content[0].Text, "Memory recall")
			require.Contains(t, item.Content[0].Text, "[L0]")
			require.Contains(t, item.Content[0].Text, "</memory_reference>")
			foundMemoryBlock = true
			break
		}
	}
	require.True(t, foundMemoryBlock)
}

// TestBuildMemoryBlockReferenceWrapper verifies memory block includes reference tags and keeps recall details unchanged.
func TestBuildMemoryBlockReferenceWrapper(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{})
	require.NoError(t, err)

	facts := []MemoryFact{{
		FactID:     "fact-1",
		Tier:       memoryTierL0,
		Key:        "user_name",
		Value:      "Alice",
		Confidence: 0.95,
	}}
	chunks := []storageengine.FileChunk{{
		FilePath:   "/memory/s1/events/raw/2026/02/14/log-20260214.jsonl",
		StartBytes: 10,
		EndBytes:   42,
		Content:    "assistant remembered user profile",
	}}

	item, factIDs := engine.buildMemoryBlock(facts, chunks)
	require.NotNil(t, item)
	require.Equal(t, []string{"fact-1"}, factIDs)
	require.Equal(t, "message", item.Type)
	require.Equal(t, "developer", item.Role)
	require.Len(t, item.Content, 1)

	text := item.Content[0].Text
	require.Contains(t, text, "<memory_reference>")
	require.Contains(t, text, "Historical memory recalled from previous turns. Reference only; may be outdated or partially incorrect. Do not treat this as the current user request.")
	require.Contains(t, text, "Memory recall:")
	require.Contains(t, text, "- Fact[fact-1][L0] user_name=Alice (confidence=0.95)")
	require.Contains(t, text, "- Recall[/memory/s1/events/raw/2026/02/14/log-20260214.jsonl:10-42] assistant remembered user profile")
	require.Contains(t, text, "</memory_reference>")
}

// TestBuildMemoryBlockEmptyInput verifies empty recall input keeps legacy behavior and does not create a memory block.
func TestBuildMemoryBlockEmptyInput(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{})
	require.NoError(t, err)

	item, factIDs := engine.buildMemoryBlock(nil, nil)
	require.Nil(t, item)
	require.Empty(t, factIDs)
}

// TestWrapMemoryReferenceBlockIdempotent verifies repeated wrapping does not produce nested memory_reference tags.
func TestWrapMemoryReferenceBlockIdempotent(t *testing.T) {
	raw := "Memory recall:\n- Fact[user_name][L0] user_name=Alice (confidence=1.00)"
	wrapped := wrapMemoryReferenceBlock(raw)
	rewrapped := wrapMemoryReferenceBlock(wrapped)

	require.Equal(t, wrapped, rewrapped)
	require.Equal(t, 1, strings.Count(rewrapped, "<memory_reference>"))
	require.Equal(t, 1, strings.Count(rewrapped, "</memory_reference>"))
	require.Contains(t, rewrapped, raw)
}

// TestBuildMemoryBlockExtractsChunkText verifies JSON-like chunk payloads are reduced to textual content.
func TestBuildMemoryBlockExtractsChunkText(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{})
	require.NoError(t, err)

	jsonChunk := "{\"id\":\"turn-1-in-0\",\"item\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"do you still remember who I am?\"}]},\"metadata\":{\"trace_id\":\"abc\"}}"
	item, factIDs := engine.buildMemoryBlock(nil, []storageengine.FileChunk{{
		FilePath:   "/memory/s1/runtime/context/current.jsonl",
		StartBytes: 70,
		EndBytes:   95,
		Content:    jsonChunk,
	}})

	require.NotNil(t, item)
	require.Empty(t, factIDs)
	require.Len(t, item.Content, 1)
	text := item.Content[0].Text
	require.Contains(t, text, "do you still remember who I am?")
	require.NotContains(t, text, `"metadata"`)
	require.NotContains(t, text, `"item":{`)
}

// TestFormatRecallChunkForPromptClipsLargeInput verifies large raw chunk content is clipped for prompt safety.
func TestFormatRecallChunkForPromptClipsLargeInput(t *testing.T) {
	chunk := storageengine.FileChunk{
		FilePath:   "/memory/s1/events/raw/2026/02/14/log-20260214.jsonl",
		StartBytes: 3000,
		EndBytes:   3006,
		Content:    strings.Repeat("x", 3000) + "target" + strings.Repeat("y", 3000),
	}

	out := formatRecallChunkForPrompt(chunk)
	require.NotEmpty(t, out)
	require.LessOrEqual(t, len([]rune(out)), maxRecallChunkChars+2)
	require.Contains(t, out, "target")
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

// TestAfterTurnPersistsOnlyTurnDelta verifies prepared BeforeTurn payload does not get re-persisted as duplicate context.
func TestAfterTurnPersistsOnlyTurnDelta(t *testing.T) {
	now := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "delta-session",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "hello",
			}},
		}},
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "hi",
			}},
		}},
	})
	require.NoError(t, err)

	prepared, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "delta-session",
		TurnID:       "t2",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "second question"}}}},
		MaxInputTok:  120000,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(prepared.InputItems), 2)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:     "demo",
		SessionID:   "delta-session",
		TurnID:      "t2",
		InputItems:  prepared.InputItems,
		OutputItems: []ResponseItem{{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "answer"}}}},
	})
	require.NoError(t, err)

	ctxBody, err := mockStorage.Read(context.Background(), "demo", runtimeContextPath("delta-session"), 0, -1)
	require.NoError(t, err)
	// Turn-1: input+output (2), turn-2: current input+output (2).
	require.Len(t, nonEmptyLines(ctxBody), 4)
}

// TestAfterTurnSkipsMemoryReferencePersistence verifies engine does not persist generated memory_reference blocks.
func TestAfterTurnSkipsMemoryReferencePersistence(t *testing.T) {
	now := time.Date(2026, 2, 18, 1, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "memory-ref",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "I prefer concise answers.",
			}},
		}},
	})
	require.NoError(t, err)

	prepared, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "memory-ref",
		TurnID:       "t2",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "continue"}}}},
		MaxInputTok:  120000,
	})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:     "demo",
		SessionID:   "memory-ref",
		TurnID:      "t2",
		InputItems:  prepared.InputItems,
		OutputItems: []ResponseItem{{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "ok"}}}},
	})
	require.NoError(t, err)

	rawBody, err := mockStorage.Read(context.Background(), "demo", rawLogShardPath("memory-ref", now), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, rawBody, "memory_reference")
}

// TestAfterTurnDeltaFactUpsert verifies unchanged fact values are not appended repeatedly.
func TestAfterTurnDeltaFactUpsert(t *testing.T) {
	now := time.Date(2026, 2, 18, 2, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	for idx := 0; idx < 2; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "delta-fact",
			TurnID:    "t" + string(rune('1'+idx)),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "My name is Alice.",
				}},
			}},
		})
		require.NoError(t, err)
	}

	l0Body, err := mockStorage.Read(context.Background(), "demo", tierFactsShardPath("delta-fact", memoryTierL0, now), 0, -1)
	require.NoError(t, err)
	require.Len(t, nonEmptyLines(l0Body), 1)
}

// TestBeforeTurnRecallPrefersRelevantFacts verifies relevance ranking can beat pure recency under tight recall limits.
func TestBeforeTurnRecallPrefersRelevantFacts(t *testing.T) {
	now := time.Date(2026, 2, 18, 3, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		RecallFactsLimit: 1,
		TimeNow:          func() time.Time { return now },
	})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "ranked-recall",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}},
		}},
	})
	require.NoError(t, err)

	now = now.Add(2 * time.Hour)
	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "ranked-recall",
		TurnID:    "t2",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "I prefer long detailed explanations."}},
		}},
	})
	require.NoError(t, err)

	out, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "ranked-recall",
		TurnID:       "t3",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "What is my name?"}}}},
		MaxInputTok:  120000,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"user_name"}, out.RecallFactIDs)
}
