package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// evaluateCallerHistoryReconciliationScenario measures duplicate-free prompt assembly and persistence trimming.
func evaluateCallerHistoryReconciliationScenario(t *testing.T, factory evalEngineFactory) map[string]float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 11, 30, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{TimeNow: clock.Now})
	require.NoError(t, err)

	historyInput := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice. I prefer concise answers."}}}
	historyOutput := ResponseItem{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "Stored."}}}
	currentInput := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "What is my preference?"}}}

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:     "demo",
		SessionID:   "eval-history-reconcile",
		TurnID:      "turn-1",
		InputItems:  []ResponseItem{historyInput},
		OutputItems: []ResponseItem{historyOutput},
	})
	require.NoError(t, err)

	prepared, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:           "demo",
		SessionID:         "eval-history-reconcile",
		TurnID:            "turn-2",
		ConversationItems: []ResponseItem{historyInput, historyOutput, currentInput},
		CurrentInputStart: 2,
		CurrentInputCount: 1,
		MaxInputTok:       120000,
	})
	require.NoError(t, err)

	duplicateCount := 0
	seen := make(map[string]struct{}, len(prepared.InputItems))
	for _, item := range prepared.InputItems {
		if isMemoryReferenceItem(item) {
			continue
		}
		identity := responseItemIdentity(item)
		if _, exists := seen[identity]; exists {
			duplicateCount++
			continue
		}
		seen[identity] = struct{}{}
	}

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:           "demo",
		SessionID:         "eval-history-reconcile",
		TurnID:            "turn-2",
		ConversationItems: []ResponseItem{historyInput, historyOutput, currentInput},
		CurrentInputStart: 2,
		CurrentInputCount: 1,
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "You prefer concise answers."}},
		}},
	})
	require.NoError(t, err)

	ctxBody, err := storage.Read(context.Background(), "demo", runtimeContextPath("eval-history-reconcile"), 0, -1)
	require.NoError(t, err)
	persistedHistoryEchoRate := 0.0
	if len(nonEmptyLines(ctxBody)) != 4 {
		persistedHistoryEchoRate = 1.0
	}

	promptDuplicateRate := 0.0
	preparedCount := max(1, len(prepared.InputItems))
	if duplicateCount > 0 {
		promptDuplicateRate = float64(duplicateCount) / float64(preparedCount)
	}

	return map[string]float64{
		"prompt_duplicate_rate":       promptDuplicateRate,
		"persisted_history_echo_rate": persistedHistoryEchoRate,
	}
}

// evaluateHistoryEquivalenceScenario measures semantic parity between latest-only and full-history requests.
func evaluateHistoryEquivalenceScenario(t *testing.T, factory evalEngineFactory) map[string]float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 11, 45, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{TimeNow: clock.Now})
	require.NoError(t, err)

	historyInput := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice. I prefer concise answers."}}}
	historyOutput := ResponseItem{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "Stored."}}}

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:     "demo",
		SessionID:   "eval-history-equivalence",
		TurnID:      "turn-1",
		InputItems:  []ResponseItem{historyInput},
		OutputItems: []ResponseItem{historyOutput},
	})
	require.NoError(t, err)

	latestOnly, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-history-equivalence",
		TurnID:    "turn-2-latest",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "What is my preference?"}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)

	fullHistory, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:           "demo",
		SessionID:         "eval-history-equivalence",
		TurnID:            "turn-2-full",
		ConversationItems: []ResponseItem{historyInput, historyOutput, {Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "What is my preference?"}}}},
		CurrentInputStart: 2,
		CurrentInputCount: 1,
		MaxInputTok:       120000,
	})
	require.NoError(t, err)

	latestSet := make(map[string]struct{}, len(latestOnly.RecallFactIDs))
	for _, factID := range latestOnly.RecallFactIDs {
		latestSet[factID] = struct{}{}
	}
	fullSet := make(map[string]struct{}, len(fullHistory.RecallFactIDs))
	for _, factID := range fullHistory.RecallFactIDs {
		fullSet[factID] = struct{}{}
	}

	score := 1.0
	if len(latestSet) != len(fullSet) {
		score = 0.0
	} else {
		for factID := range latestSet {
			if _, exists := fullSet[factID]; !exists {
				score = 0.0
				break
			}
		}
	}

	return map[string]float64{"history_equivalence_score": score}
}

// evaluateRecallAndRetentionScenario measures targeted recall quality and expiry correctness.
func evaluateRecallAndRetentionScenario(t *testing.T, factory evalEngineFactory) map[string]float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{
		RecentContextItems: 5,
		RecallFactsLimit:   2,
		SearchLimit:        5,
		TimeNow:            clock.Now,
	})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "eval-recall",
		TurnID:    "turn-1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "My name is Alice. I prefer concise answers. Today I need finish the monthly report.",
			}},
		}},
	})
	require.NoError(t, err)

	initialOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-recall",
		TurnID:    "turn-2",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "What is my name and what do I prefer?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)

	expected := map[string]struct{}{
		"user_name":       {},
		"user_preference": {},
	}
	matched := 0
	for _, factID := range initialOut.RecallFactIDs {
		if _, ok := expected[factID]; ok {
			matched++
		}
	}

	precision := safeRatio(matched, len(initialOut.RecallFactIDs))
	recall := safeRatio(matched, len(expected))

	clock.Advance(48 * time.Hour)
	err = engine.RunMaintenance(context.Background(), "demo", "eval-recall")
	require.NoError(t, err)

	postMaintenanceOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-recall",
		TurnID:    "turn-3",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "Who am I and what do I need today?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)

	durableSurvival := 0.0
	if contains(postMaintenanceOut.RecallFactIDs, "user_name") {
		durableSurvival = 1.0
	}

	expiredSuppression := 1.0
	if contains(postMaintenanceOut.RecallFactIDs, "today_task") {
		expiredSuppression = 0.0
	}

	return map[string]float64{
		"fact_recall_precision":         precision,
		"fact_recall_recall":            recall,
		"durable_fact_survival_rate":    durableSurvival,
		"expired_fact_suppression_rate": expiredSuppression,
	}
}

// evaluateIdempotencyScenario measures whether replaying one turn ID duplicates persistence.
func evaluateIdempotencyScenario(t *testing.T, factory evalEngineFactory) float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{TimeNow: clock.Now})
	require.NoError(t, err)

	input := AfterTurnInput{
		Project:   "demo",
		SessionID: "eval-idempotent",
		TurnID:    "turn-1",
		InputItems: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}},
		}},
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "Noted."}},
		}},
	}

	err = engine.AfterTurn(context.Background(), input)
	require.NoError(t, err)
	err = engine.AfterTurn(context.Background(), input)
	require.NoError(t, err)

	rawBody, err := storage.Read(context.Background(), "demo", rawLogShardPath("eval-idempotent", clock.Now()), 0, -1)
	require.NoError(t, err)
	ctxBody, err := storage.Read(context.Background(), "demo", runtimeContextPath("eval-idempotent"), 0, -1)
	require.NoError(t, err)

	if len(nonEmptyLines(rawBody)) == 2 && len(nonEmptyLines(ctxBody)) == 2 {
		return 1.0
	}

	return 0.0
}

// evaluateDuplicateGrowthScenario measures the approximate-dedup weakness under scaled fact volume.
func evaluateDuplicateGrowthScenario(t *testing.T, factory evalEngineFactory) map[string]float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 11, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{
		RecallFactsLimit: 2,
		TimeNow:          clock.Now,
		HeuristicClient:  scaleHeuristicClient{DistractorCount: 5},
	})
	require.NoError(t, err)

	for idx := 0; idx < 6; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "eval-duplicate-growth",
			TurnID:    fmt.Sprintf("turn-%d", idx),
			InputItems: []ResponseItem{{
				Type:    "message",
				Role:    "user",
				Content: []ResponseContentPart{{Type: "input_text", Text: "memory signal batch"}},
			}},
		})
		require.NoError(t, err)
		clock.Advance(time.Minute)
	}

	writes := countFactWritesByFactID(t, engine, "demo", "eval-duplicate-growth", "stable_preference")
	return map[string]float64{
		"duplicate_growth_ratio": float64(writes),
		"exact_dedup_score":      safeRatio(1, writes),
	}
}

// evaluateSessionIsolationScenario measures whether recall leaks across sessions by default.
func evaluateSessionIsolationScenario(t *testing.T, factory evalEngineFactory) float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{TimeNow: clock.Now})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "eval-isolation-a",
		TurnID:    "turn-a1",
		InputItems: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}},
		}},
	})
	require.NoError(t, err)

	out, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-isolation-b",
		TurnID:    "turn-b1",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "What is my name?"}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)

	if contains(out.RecallFactIDs, "user_name") {
		return 1.0
	}

	return 0.0
}

// evaluateCompactionScenario measures whether bounded-context compaction still triggers when needed.
func evaluateCompactionScenario(t *testing.T, factory evalEngineFactory) float64 {
	t.Helper()

	clock := newEvalClock(time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := factory(storage, Config{
		RecentContextItems: 2,
		CompactThreshold:   0.8,
		TimeNow:            clock.Now,
	})
	require.NoError(t, err)

	for idx := 0; idx < 6; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "eval-compaction",
			TurnID:    fmt.Sprintf("turn-%d", idx),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "This is a long memory sentence that should force compaction when the prompt budget is small.",
				}},
			}},
		})
		require.NoError(t, err)
	}

	_, err = engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-compaction",
		TurnID:    "turn-final",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "continue"}},
		}},
		MaxInputTok: 10,
	})
	require.NoError(t, err)

	body, err := storage.Read(context.Background(), "demo", runtimeContextPath("eval-compaction"), 0, -1)
	require.NoError(t, err)

	if strings.Contains(body, "\"type\":\"compact_summary\"") {
		return 1.0
	}

	return 0.0
}
