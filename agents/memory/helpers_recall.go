package memory

import (
	"sort"
	"strings"
	"time"
)

// deduplicateFacts keeps the first entry per fact identity and returns deduplicated list.
//
// Parameters:
//   - facts: Candidate facts already sorted by preferred order.
//
// Returns:
//   - Deduplicated facts where identity is fact_id + key.
func deduplicateFacts(facts []MemoryFact) []MemoryFact {
	seen := make(map[string]struct{}, len(facts))
	result := make([]MemoryFact, 0, len(facts))
	for _, fact := range facts {
		identity := factIdentity(fact)
		if identity == "" {
			continue
		}
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, fact)
	}

	return result
}

// rankFactsForRecall ranks facts by relevance, recency, and confidence and returns sorted facts.
//
// Parameters:
//   - now: Current UTC time for recency scoring.
//   - facts: Candidate active facts.
//   - query: Current turn query text for relevance matching.
//
// Returns:
//   - Facts sorted from highest to lowest recall priority.
func rankFactsForRecall(now time.Time, facts []MemoryFact, query string) []MemoryFact {
	if len(facts) == 0 {
		return nil
	}

	queryTerms := tokenizeRecallQuery(query)
	type rankedFact struct {
		fact  MemoryFact
		score float64
	}

	ranked := make([]rankedFact, 0, len(facts))
	for _, fact := range facts {
		ranked = append(ranked, rankedFact{fact: fact, score: computeRecallScore(now, fact, queryTerms)})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].fact.TS == ranked[j].fact.TS {
				return ranked[i].fact.Confidence > ranked[j].fact.Confidence
			}
			return ranked[i].fact.TS > ranked[j].fact.TS
		}
		return ranked[i].score > ranked[j].score
	})

	result := make([]MemoryFact, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.fact)
	}

	return result
}

// factIdentity builds a stable identity key from one fact.
//
// Parameters:
//   - fact: Source fact.
//
// Returns:
//   - A normalized identity string, or empty when identity fields are missing.
func factIdentity(fact MemoryFact) string {
	factID := strings.TrimSpace(strings.ToLower(fact.FactID))
	field := strings.TrimSpace(strings.ToLower(fact.Key))
	if factID == "" && field == "" {
		return ""
	}

	return factID + "::" + field
}

// normalizeFactValue normalizes free-form fact values for dedup comparison.
//
// Parameters:
//   - value: Original fact value.
//
// Returns:
//   - A lowercase, compacted value suitable for equality checks.
func normalizeFactValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

// computeRecallScore computes fact recall priority score from confidence, recency, tier, and relevance.
//
// Parameters:
//   - now: Current UTC time.
//   - fact: Fact to score.
//   - queryTerms: Normalized query terms.
//
// Returns:
//   - Composite score where higher values indicate stronger recall priority.
func computeRecallScore(now time.Time, fact MemoryFact, queryTerms []string) float64 {
	score := fact.Confidence

	switch fact.Tier {
	case memoryTierL0:
		score += 0.35
	case memoryTierL1:
		score += 0.10
	case memoryTierL2:
		score += 0.20
	}

	if ts, err := time.Parse(time.RFC3339, fact.TS); err == nil {
		hours := now.UTC().Sub(ts.UTC()).Hours()
		if hours < 0 {
			hours = 0
		}
		score += maxFloat64(0, 0.25-hours/240)
	}

	if len(queryTerms) > 0 {
		haystack := strings.ToLower(strings.TrimSpace(fact.Key + " " + fact.Value + " " + fact.FactID))
		matched := 0
		for _, term := range queryTerms {
			if strings.Contains(haystack, term) {
				matched++
			}
		}
		if matched > 0 {
			score += 0.25 + float64(matched)*0.15
		}
	}

	return score
}

// tokenizeRecallQuery tokenizes query text into useful lowercase terms for matching.
//
// Parameters:
//   - query: Raw query text.
//
// Returns:
//   - Deduplicated lowercase terms with short stop words removed.
func tokenizeRecallQuery(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return nil
	}

	replacer := strings.NewReplacer(
		"\n", " ",
		"\t", " ",
		"?", " ",
		"!", " ",
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"\"", " ",
		"'", " ",
	)
	normalized = replacer.Replace(normalized)

	stopWords := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {},
		"is": {}, "are": {}, "am": {}, "to": {}, "of": {}, "my": {},
		"what": {}, "who": {}, "do": {}, "you": {}, "i": {},
	}

	seen := make(map[string]struct{})
	terms := make([]string, 0, 8)
	for _, token := range strings.Fields(normalized) {
		if len(token) < 2 {
			continue
		}
		if _, ok := stopWords[token]; ok {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		terms = append(terms, token)
	}

	return terms
}

// maxFloat64 returns the larger value between a and b.
//
// Parameters:
//   - a: First float value.
//   - b: Second float value.
//
// Returns:
//   - The larger value.
func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}

	return b
}
