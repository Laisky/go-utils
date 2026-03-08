package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// evalEngineFactory builds one engine instance for quantitative evaluation.
type evalEngineFactory func(storage storageengine.Engine, conf Config) (*StandardEngine, error)

// evalClock provides deterministic time control for quantitative scenarios.
type evalClock struct {
	now time.Time
}

// newEvalClock creates one deterministic evaluation clock.
func newEvalClock(now time.Time) *evalClock {
	return &evalClock{now: now.UTC()}
}

// Now returns the current evaluation time in UTC.
func (clock *evalClock) Now() time.Time {
	return clock.now.UTC()
}

// Advance moves the evaluation clock forward by a duration.
func (clock *evalClock) Advance(delta time.Duration) {
	clock.now = clock.now.Add(delta).UTC()
}

// quantitativeEvalResult stores measurable outputs for one engine run.
type quantitativeEvalResult struct {
	Metrics map[string]float64
}

// newQuantitativeEvalResult creates an empty result container.
func newQuantitativeEvalResult() quantitativeEvalResult {
	return quantitativeEvalResult{Metrics: make(map[string]float64)}
}

// Set stores one named metric value.
func (result *quantitativeEvalResult) Set(name string, value float64) {
	result.Metrics[name] = value
}

// Get returns one metric value or zero when unset.
func (result quantitativeEvalResult) Get(name string) float64 {
	return result.Metrics[name]
}

// Log writes metrics in deterministic order for baseline review.
func (result quantitativeEvalResult) Log(t *testing.T) {
	t.Helper()

	keys := make([]string, 0, len(result.Metrics))
	for key := range result.Metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		t.Logf("%s=%.4f", key, result.Metrics[key])
	}
}

// quantitativeGate describes one acceptance criterion for V1-vs-V2 comparison.
type quantitativeGate struct {
	Name        string
	Direction   string
	TargetValue float64
}

// quantitativeImprovementRule describes how a candidate should compare against baseline.
type quantitativeImprovementRule struct {
	Name      string
	Direction string
}

// defaultQuantitativeGates returns the recommended V2 acceptance gates.
func defaultQuantitativeGates() []quantitativeGate {
	return []quantitativeGate{
		{Name: "fact_recall_recall", Direction: ">=", TargetValue: 1.00},
		{Name: "fact_recall_precision", Direction: ">=", TargetValue: 1.00},
		{Name: "expired_fact_suppression_rate", Direction: ">=", TargetValue: 1.00},
		{Name: "durable_fact_survival_rate", Direction: ">=", TargetValue: 1.00},
		{Name: "idempotency_score", Direction: ">=", TargetValue: 1.00},
		{Name: "session_isolation_leak_rate", Direction: "<=", TargetValue: 0.00},
		{Name: "compaction_guard_score", Direction: ">=", TargetValue: 1.00},
		{Name: "exact_dedup_score", Direction: ">=", TargetValue: 0.95},
		{Name: "duplicate_growth_ratio", Direction: "<=", TargetValue: 1.05},
	}
}

// defaultQuantitativeImprovementRules returns the V2-vs-V1 comparison rules.
func defaultQuantitativeImprovementRules() []quantitativeImprovementRule {
	return []quantitativeImprovementRule{
		{Name: "fact_recall_recall", Direction: ">="},
		{Name: "fact_recall_precision", Direction: ">="},
		{Name: "expired_fact_suppression_rate", Direction: ">="},
		{Name: "durable_fact_survival_rate", Direction: ">="},
		{Name: "idempotency_score", Direction: ">="},
		{Name: "session_isolation_leak_rate", Direction: "<="},
		{Name: "compaction_guard_score", Direction: ">="},
		{Name: "exact_dedup_score", Direction: ">="},
		{Name: "duplicate_growth_ratio", Direction: "<="},
	}
}

// assertQuantitativeGates verifies one result against acceptance thresholds.
func assertQuantitativeGates(t *testing.T, result quantitativeEvalResult, gates []quantitativeGate) {
	t.Helper()

	violations := quantitativeGateViolations(result, gates)
	require.Empty(t, violations, "quantitative gate violations: %v", violations)
}

// quantitativeGateViolations returns all unmet gate descriptions for one result set.
func quantitativeGateViolations(result quantitativeEvalResult, gates []quantitativeGate) []string {
	violations := make([]string, 0)
	for _, gate := range gates {
		value := result.Get(gate.Name)
		switch gate.Direction {
		case ">=":
			if value < gate.TargetValue {
				violations = append(violations, fmt.Sprintf("%s %.4f < %.4f", gate.Name, value, gate.TargetValue))
			}
		case "<=":
			if value > gate.TargetValue {
				violations = append(violations, fmt.Sprintf("%s %.4f > %.4f", gate.Name, value, gate.TargetValue))
			}
		default:
			violations = append(violations, fmt.Sprintf("%s unsupported direction %s", gate.Name, gate.Direction))
		}
	}

	return violations
}

// quantitativeImprovementViolations returns unmet baseline-improvement expectations for a candidate result.
func quantitativeImprovementViolations(
	baseline, candidate quantitativeEvalResult,
	rules []quantitativeImprovementRule,
) []string {
	violations := make([]string, 0)
	for _, rule := range rules {
		baselineValue := baseline.Get(rule.Name)
		candidateValue := candidate.Get(rule.Name)
		switch rule.Direction {
		case ">=":
			if candidateValue < baselineValue {
				violations = append(violations, fmt.Sprintf("%s %.4f < baseline %.4f", rule.Name, candidateValue, baselineValue))
			}
		case "<=":
			if candidateValue > baselineValue {
				violations = append(violations, fmt.Sprintf("%s %.4f > baseline %.4f", rule.Name, candidateValue, baselineValue))
			}
		default:
			violations = append(violations, fmt.Sprintf("%s unsupported direction %s", rule.Name, rule.Direction))
		}
	}

	return violations
}

// runQuantitativeEvaluationSuite runs the deterministic baseline scenarios for one engine factory.
func runQuantitativeEvaluationSuite(t *testing.T, factory evalEngineFactory) quantitativeEvalResult {
	t.Helper()

	result := newQuantitativeEvalResult()
	for key, value := range evaluateRecallAndRetentionScenario(t, factory) {
		result.Set(key, value)
	}
	result.Set("idempotency_score", evaluateIdempotencyScenario(t, factory))
	for key, value := range evaluateDuplicateGrowthScenario(t, factory) {
		result.Set(key, value)
	}
	result.Set("session_isolation_leak_rate", evaluateSessionIsolationScenario(t, factory))
	result.Set("compaction_guard_score", evaluateCompactionScenario(t, factory))

	return result
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
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}},
		}},
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
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
				Type: "message",
				Role: "user",
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
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice."}},
		}},
	})
	require.NoError(t, err)

	out, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "eval-isolation-b",
		TurnID:    "turn-b1",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
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
			Type: "message",
			Role: "user",
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

// countFactWritesByFactID counts persisted fact records across all tiers for one fact_id.
func countFactWritesByFactID(
	t *testing.T,
	engine *StandardEngine,
	project, sessionID, factID string,
) int {
	t.Helper()

	count := 0
	for _, tier := range []string{memoryTierL0, memoryTierL1, memoryTierL2} {
		facts, err := engine.loadTierFacts(context.Background(), project, sessionID, tier)
		require.NoError(t, err)
		for _, fact := range facts {
			if fact.FactID == factID {
				count++
			}
		}
	}

	return count
}

// safeRatio returns zero when denominator is zero.
func safeRatio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}

	return float64(numerator) / float64(denominator)
}

// scaleHeuristicClient generates one stable fact plus many distractors to expose approximate dedup.
type scaleHeuristicClient struct {
	DistractorCount int
}

// ExtractAndMergeFacts returns a stable identity fact plus high-relevance distractors.
func (client scaleHeuristicClient) ExtractAndMergeFacts(_ context.Context, in HeuristicFactInput) ([]MemoryFact, error) {
	facts := []MemoryFact{{
		ID:         in.TurnID + "-stable",
		TS:         in.NowRFC3339,
		Type:       "fact_upsert",
		FactID:     "stable_preference",
		Key:        "preference",
		Value:      "concise",
		Confidence: 0.55,
		Tier:       memoryTierL0,
	}}

	for idx := 0; idx < client.DistractorCount; idx++ {
		facts = append(facts, MemoryFact{
			ID:         fmt.Sprintf("%s-distractor-%d", in.TurnID, idx),
			TS:         in.NowRFC3339,
			Type:       "fact_upsert",
			FactID:     fmt.Sprintf("distractor_%s_%d", in.TurnID, idx),
			Key:        fmt.Sprintf("memory_%d", idx),
			Value:      "memory signal batch",
			Confidence: 0.98,
			Tier:       memoryTierL2,
		})
	}

	return facts, nil
}
