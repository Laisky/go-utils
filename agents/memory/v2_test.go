package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBeforeTurnConversationHistoryDeduplicatesRecentContext verifies caller-supplied history suppresses duplicate engine recall.
func TestBeforeTurnConversationHistoryDeduplicatesRecentContext(t *testing.T) {
	now := time.Date(2026, 3, 8, 17, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	historyInput := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}}}
	historyOutput := ResponseItem{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "Stored."}}}
	currentInput := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "What is my name?"}}}

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:     "demo",
		SessionID:   "v2-history",
		TurnID:      "turn-1",
		InputItems:  []ResponseItem{historyInput},
		OutputItems: []ResponseItem{historyOutput},
	})
	require.NoError(t, err)

	prepared, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:           "demo",
		SessionID:         "v2-history",
		TurnID:            "turn-2",
		ConversationItems: []ResponseItem{historyInput, historyOutput, currentInput},
		CurrentInputStart: 2,
		CurrentInputCount: 1,
		MaxInputTok:       120000,
	})
	require.NoError(t, err)

	seen := make(map[string]struct{}, len(prepared.InputItems))
	for _, item := range prepared.InputItems {
		if isMemoryReferenceItem(item) {
			continue
		}
		identity := responseItemIdentity(item)
		_, exists := seen[identity]
		require.False(t, exists)
		seen[identity] = struct{}{}
	}
	require.Contains(t, prepared.RecallFactIDs, "user_name")
}

// TestAfterTurnUsesActiveFactIndexForExactDedup verifies write-side dedup remains exact under capped recall pressure.
func TestAfterTurnUsesActiveFactIndexForExactDedup(t *testing.T) {
	now := time.Date(2026, 3, 8, 17, 30, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{
		TimeNow:          func() time.Time { return now },
		RecallFactsLimit: 2,
		HeuristicClient:  scaleHeuristicClient{DistractorCount: 5},
	})
	require.NoError(t, err)

	for idx := 0; idx < 4; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "v2-dedup",
			TurnID:    benchmarkTurnID(idx),
			InputItems: []ResponseItem{{
				Type:    "message",
				Role:    "user",
				Content: []ResponseContentPart{{Type: "input_text", Text: "memory signal batch"}},
			}},
		})
		require.NoError(t, err)
	}

	count := countFactWritesByFactID(t, engine, "demo", "v2-dedup", "stable_preference")
	require.Equal(t, 1, count)

	activeIndex, err := engine.loadActiveFactsIndex(context.Background(), "demo", "v2-dedup")
	require.NoError(t, err)
	require.NotEmpty(t, activeIndex.Facts)
	_, exists := activeIndex.Facts["stable_preference::preference"]
	require.True(t, exists)
}

// TestRunConsolidationWritesInsightAndRecallIncludesIt verifies maintenance builds insights that BeforeTurn can recall.
func TestRunConsolidationWritesInsightAndRecallIncludesIt(t *testing.T) {
	now := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }, ConsolidationMinEvents: 2})
	require.NoError(t, err)

	for idx, text := range []string{
		"I prefer concise answers.",
		"I prefer concise answers for status updates.",
	} {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "v2-insight",
			TurnID:    benchmarkTurnID(idx),
			InputItems: []ResponseItem{{
				Type:    "message",
				Role:    "user",
				Content: []ResponseContentPart{{Type: "input_text", Text: text}},
			}},
		})
		require.NoError(t, err)
	}

	err = engine.RunConsolidation(context.Background(), "demo", "v2-insight")
	require.NoError(t, err)

	insights, err := engine.loadInsights(context.Background(), "demo", "v2-insight")
	require.NoError(t, err)
	require.NotEmpty(t, insights)

	prepared, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "v2-insight",
		TurnID:    "turn-final",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "What should you remember about my preference?"}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, prepared.RecallInsightIDs)
}
