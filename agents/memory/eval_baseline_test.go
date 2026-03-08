package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMemoryQuantitativeEvaluationBaseline captures the measurable V1 baseline.
//
// This test does not claim the current engine is ideal. It captures:
// 1. Guardrails that V2 must not regress.
// 2. Known weaknesses that V2 is expected to improve.
func TestMemoryQuantitativeEvaluationBaseline(t *testing.T) {
	result := runQuantitativeEvaluationSuite(t, NewEngine)
	result.Log(t)

	// Guardrails that already work well in V1 and must stay strong in V2.
	require.InDelta(t, 1.0, result.Get("fact_recall_recall"), 0.0001)
	require.InDelta(t, 1.0, result.Get("fact_recall_precision"), 0.0001)
	require.InDelta(t, 1.0, result.Get("durable_fact_survival_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("expired_fact_suppression_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("idempotency_score"), 0.0001)
	require.InDelta(t, 0.0, result.Get("session_isolation_leak_rate"), 0.0001)
	require.InDelta(t, 1.0, result.Get("compaction_guard_score"), 0.0001)

	// Known weakness in V1: dedup is approximate under scaled fact volume.
	require.Greater(t, result.Get("duplicate_growth_ratio"), 1.0)
	require.Less(t, result.Get("exact_dedup_score"), 1.0)
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

// TestQuantitativeGatesRejectCurrentV1 proves the current engine does not yet satisfy the full V2 gates.
func TestQuantitativeGatesRejectCurrentV1(t *testing.T) {
	result := runQuantitativeEvaluationSuite(t, NewEngine)
	violations := quantitativeGateViolations(result, defaultQuantitativeGates())
	require.NotEmpty(t, violations)

	foundDuplicateGrowthViolation := false
	for _, violation := range violations {
		if strings.Contains(violation, "duplicate_growth_ratio") || strings.Contains(violation, "exact_dedup_score") {
			foundDuplicateGrowthViolation = true
			break
		}
	}
	require.True(t, foundDuplicateGrowthViolation)
}
