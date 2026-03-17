package memory

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// factMutationPlan stores exact fact writes plus resulting active-index state.
type factMutationPlan struct {
	Writes              []MemoryFact
	NextActiveFacts     map[string]MemoryFact
	DedupeSkipCount     int
	DeletedFactCount    int
	SupersededFactCount int
}

// selectExactFactMutations plans exact fact writes against the full active index.
func selectExactFactMutations(
	now time.Time,
	turnID, userID string,
	activeFacts map[string]MemoryFact,
	candidates []MemoryFact,
	deletedFactIDs []string,
) factMutationPlan {
	plan := factMutationPlan{
		Writes:          make([]MemoryFact, 0, len(candidates)+len(deletedFactIDs)),
		NextActiveFacts: cloneFactMap(activeFacts),
	}
	nowRFC3339 := now.UTC().Format(time.RFC3339)

	for _, factID := range deduplicateStrings(deletedFactIDs) {
		matches := activeFactsByFactID(plan.NextActiveFacts, factID)
		if len(matches) == 0 {
			continue
		}
		for _, existing := range matches {
			delete(plan.NextActiveFacts, factIdentity(existing))
			plan.Writes = append(plan.Writes, buildFactStateRecord(
				existing,
				nowRFC3339,
				turnID,
				userID,
				"fact_delete",
				memoryStateDeleted,
				"",
			))
			plan.DeletedFactCount++
		}
	}

	orderedCandidates := deduplicateCandidateFacts(candidates)
	for _, candidate := range orderedCandidates {
		identity := factIdentity(candidate)
		if identity == "" {
			continue
		}

		existing, ok := plan.NextActiveFacts[identity]
		if ok &&
			!isFactExpired(now, existing) &&
			normalizeFactValue(existing.Value) == normalizeFactValue(candidate.Value) &&
			existing.Tier == candidate.Tier {
			plan.DedupeSkipCount++
			continue
		}

		candidate.State = memoryStateActive
		candidate.SourceTurnID = turnID
		candidate.SourceUserID = userID
		candidate.TS = nowRFC3339

		if ok && !isFactExpired(now, existing) {
			plan.Writes = append(plan.Writes, buildFactStateRecord(
				existing,
				nowRFC3339,
				turnID,
				userID,
				"fact_supersede",
				memoryStateSuperseded,
				candidate.ID,
			))
			plan.SupersededFactCount++
		}

		plan.Writes = append(plan.Writes, candidate)
		plan.NextActiveFacts[identity] = candidate
	}

	return plan
}

// deduplicateCandidateFacts keeps the latest candidate for each fact identity.
func deduplicateCandidateFacts(candidates []MemoryFact) []MemoryFact {
	if len(candidates) == 0 {
		return nil
	}

	latest := make(map[string]MemoryFact, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		identity := factIdentity(candidate)
		if identity == "" {
			continue
		}
		if _, exists := latest[identity]; !exists {
			order = append(order, identity)
		}
		latest[identity] = candidate
	}

	result := make([]MemoryFact, 0, len(latest))
	for _, identity := range order {
		result = append(result, latest[identity])
	}

	return result
}

// buildFactStateRecord creates one state-transition record derived from an existing fact.
func buildFactStateRecord(
	existing MemoryFact,
	nowRFC3339, turnID, userID, recordType, state, supersededBy string,
) MemoryFact {
	record := existing
	record.ID = fmt.Sprintf("%s-%s-%s", turnID, recordType, strings.ReplaceAll(existing.FactID, " ", "_"))
	record.TS = nowRFC3339
	record.Type = recordType
	record.State = state
	record.SourceTurnID = turnID
	record.SourceUserID = userID
	record.SupersededBy = supersededBy
	if state == memoryStateDeleted {
		record.DeletedAt = nowRFC3339
	}

	return record
}

// cloneFactMap clones one identity-keyed fact map.
func cloneFactMap(src map[string]MemoryFact) map[string]MemoryFact {
	if len(src) == 0 {
		return make(map[string]MemoryFact)
	}

	cloned := make(map[string]MemoryFact, len(src))
	for key, value := range src {
		cloned[key] = value
	}

	return cloned
}

// activeFactsByFactID returns active facts whose fact_id matches the requested id.
func activeFactsByFactID(active map[string]MemoryFact, factID string) []MemoryFact {
	trimmed := strings.TrimSpace(strings.ToLower(factID))
	if trimmed == "" {
		return nil
	}

	matched := make([]MemoryFact, 0, 2)
	for _, fact := range active {
		if strings.TrimSpace(strings.ToLower(fact.FactID)) != trimmed {
			continue
		}
		matched = append(matched, fact)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].TS > matched[j].TS
	})

	return matched
}

// deduplicateStrings removes duplicates while preserving first-seen order.
func deduplicateStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	return result
}
