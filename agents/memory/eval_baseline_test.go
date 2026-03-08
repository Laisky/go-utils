package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMemoryQuantitativeEvaluationBaseline captures the measurable V2 baseline.
func TestMemoryQuantitativeEvaluationBaseline(t *testing.T) {
	result := runQuantitativeEvaluationSuite(t, NewEngine)
	result.Log(t)

	require.InDelta(t, 1.0, result.Get("fact_recall_recall"), 0.0001)
	require.InDelta(t, 1.0, result.Get("fact_recall_precision"), 0.0001)
	require.InDelta(t, 1.0, result.Get("durable_fact_survival_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("expired_fact_suppression_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("idempotency_score"), 0.0001)
	require.InDelta(t, 0.0, result.Get("session_isolation_leak_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("compaction_guard_score"), 0.0001)
	require.InDelta(t, 1.0, result.Get("exact_dedup_score"), 0.0001)
	require.InDelta(t, 1.0, result.Get("history_equivalence_score"), 0.0001)
	require.InDelta(t, 0.0, result.Get("prompt_duplicate_rate"), 0.0001)
	require.InDelta(t, 0.0, result.Get("persisted_history_echo_rate"), 0.0001)
}

// TestQuantitativeGateDefinitions ensures the proposed V2 acceptance gates are internally consistent.
func TestQuantitativeGateDefinitions(t *testing.T) {
	gates := defaultQuantitativeGates()
	require.NotEmpty(t, gates)

	seen := make(map[string]struct{}, len(gates))
	for _, gate := range gates {
		require.NotEmpty(t, gate.Name)
		require.NotEmpty(t, gate.Direction)
		_, exists := seen[gate.Name]
		require.False(t, exists, "duplicate gate for metric `%s`", gate.Name)
		seen[gate.Name] = struct{}{}
	}
}

// TestQuantitativeImprovementRulesAgainstSelf verifies the comparison helper is stable.
func TestQuantitativeImprovementRulesAgainstSelf(t *testing.T) {
	result := runQuantitativeEvaluationSuite(t, NewEngine)
	violations := quantitativeImprovementViolations(result, result, defaultQuantitativeImprovementRules())
	require.Empty(t, violations)
}

// TestQuantitativeGatesAcceptCurrentV2 proves the current engine satisfies the V2 gates.
func TestQuantitativeGatesAcceptCurrentV2(t *testing.T) {
	result := runQuantitativeEvaluationSuite(t, NewEngine)
	violations := quantitativeGateViolations(result, defaultQuantitativeGates())
	require.Empty(t, violations)
}
