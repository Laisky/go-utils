package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/klauspost/compress/zstd"
)

// validateSessionID ensures sessionID is safe to use as a single path segment.
//
// Security: sessionID flows raw into sessionBasePath ("/memory/"+sessionID) and
// through path.Join when building per-session storage paths. Without validation a
// sessionID such as "..", "foo/../victim", or one containing a NUL/CR/LF could
// escape the intended "/memory/<session>/" namespace and read, overwrite, or
// delete another session's data within the same project (path traversal /
// cross-session namespace escape). We forbid path separators and control
// characters (so the id is always a single segment) and explicitly reject the
// only traversal-capable single segments "." and "..". A regex like
// ^[A-Za-z0-9_.-]{1,128}$ would wrongly accept "..", so a char loop is used. This
// is maximally backward compatible: it only rejects genuinely unsafe ids.
func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return newValidationError(ValidationErrorCodeSessionIDRequired, "session_id", "session_id is required")
	}
	if len(sessionID) > 128 {
		return newValidationError(ValidationErrorCodeSessionIDInvalid, "session_id", "session_id is too long")
	}
	for _, r := range sessionID {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return newValidationError(
				ValidationErrorCodeSessionIDInvalid,
				"session_id",
				"session_id must not contain path separators or control characters",
			)
		}
	}
	if sessionID == "." || sessionID == ".." {
		return newValidationError(
			ValidationErrorCodeSessionIDInvalid,
			"session_id",
			"session_id must not be a path traversal token",
		)
	}

	return nil
}

// validateBeforeTurnInput validates required fields for BeforeTurn and returns validation error.
func validateBeforeTurnInput(in BeforeTurnInput) error {
	if strings.TrimSpace(in.Project) == "" {
		return newValidationError(ValidationErrorCodeProjectRequired, "project", "project is required")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return newValidationError(ValidationErrorCodeSessionIDRequired, "session_id", "session_id is required")
	}
	if err := validateSessionID(in.SessionID); err != nil {
		return err
	}
	if strings.TrimSpace(in.TurnID) == "" {
		return newValidationError(ValidationErrorCodeTurnIDRequired, "turn_id", "turn_id is required")
	}
	if len(in.CurrentInput) == 0 && len(in.ConversationItems) == 0 {
		return newValidationError(ValidationErrorCodeCurrentInputRequired, "current_input", "current_input is required")
	}

	return nil
}

// validateAfterTurnInput validates required fields for AfterTurn and returns validation error.
func validateAfterTurnInput(in AfterTurnInput) error {
	if strings.TrimSpace(in.Project) == "" {
		return newValidationError(ValidationErrorCodeProjectRequired, "project", "project is required")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return newValidationError(ValidationErrorCodeSessionIDRequired, "session_id", "session_id is required")
	}
	if err := validateSessionID(in.SessionID); err != nil {
		return err
	}
	if strings.TrimSpace(in.TurnID) == "" {
		return newValidationError(ValidationErrorCodeTurnIDRequired, "turn_id", "turn_id is required")
	}

	return nil
}

// extractInputText flattens textual content from response items for recall queries.
func extractInputText(items []ResponseItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
		if strings.TrimSpace(item.Output) != "" {
			parts = append(parts, item.Output)
		}
	}

	return strings.Join(parts, "\n")
}

// estimateTokens estimates token count conservatively from response items and returns estimated tokens.
func estimateTokens(items []ResponseItem) int {
	totalChars := 0
	for _, item := range items {
		totalChars += len(item.Type) + len(item.Role) + len(item.CallID) + len(item.Output)
		for _, part := range item.Content {
			totalChars += len(part.Type) + len(part.Text) + len(part.ImageURL) + len(part.FileID) + len(part.Filename)
		}
	}

	tokens := totalChars / 4
	if tokens == 0 && totalChars > 0 {
		tokens = 1
	}

	return tokens
}

// buildTurnEvents builds immutable turn-level log events and returns the appended events.
func buildTurnEvents(turnID, userID, ts string, inputItems, outputItems []ResponseItem) []LogEvent {
	events := make([]LogEvent, 0, len(inputItems)+len(outputItems))
	for idx, item := range inputItems {
		events = append(events, LogEvent{
			ID:     fmt.Sprintf("%s-in-%d", turnID, idx),
			TS:     ts,
			Type:   "input_item",
			TurnID: turnID,
			UserID: userID,
			ItemID: responseItemIdentity(item),
			Item:   item,
		})
	}
	for idx, item := range outputItems {
		events = append(events, LogEvent{
			ID:     fmt.Sprintf("%s-out-%d", turnID, idx),
			TS:     ts,
			Type:   "output_item",
			TurnID: turnID,
			UserID: userID,
			ItemID: responseItemIdentity(item),
			Item:   item,
		})
	}

	return events
}

// extractFacts extracts rule-based memory facts from input items and returns candidate facts.
func extractFacts(turnID, ts string, inputItems []ResponseItem) []MemoryFact {
	text := strings.ToLower(extractInputText(inputItems))
	facts := make([]MemoryFact, 0, 5)

	if value, ok := pickSuffix(text, "my name is "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-name",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_name",
			Key:        "name",
			Value:      value,
			Confidence: 0.95,
			Tier:       memoryTierL0,
		})
	}

	if value, ok := pickSuffix(text, "i prefer "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-prefer",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_preference",
			Key:        "preference",
			Value:      value,
			Confidence: 0.92,
			Tier:       memoryTierL0,
		})
	}

	if value, ok := pickSuffix(text, "i like "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-like",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_like",
			Key:        "like",
			Value:      value,
			Confidence: 0.85,
			Tier:       memoryTierL2,
		})
	}

	if value, ok := pickSuffix(text, "today i need "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-today-task",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "today_task",
			Key:        "today_task",
			Value:      value,
			Confidence: 0.80,
			Tier:       memoryTierL1,
		})
	}

	if value, ok := pickSuffix(text, "this week i need "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-week-task",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "weekly_task",
			Key:        "weekly_task",
			Value:      value,
			Confidence: 0.82,
			Tier:       memoryTierL2,
		})
	}

	return facts
}

// pickSuffix extracts a short value after marker and returns the value and whether extraction succeeded.
func pickSuffix(text, marker string) (string, bool) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}

	sub := text[idx+len(marker):]
	for _, sep := range []string{"\n", ".", "!", "?", ",", ";"} {
		if pos := strings.Index(sub, sep); pos >= 0 {
			sub = sub[:pos]
		}
	}

	value := strings.TrimSpace(sub)
	if value == "" {
		return "", false
	}

	if len(value) > 128 {
		value = strings.TrimSpace(value[:128])
	}

	return value, true
}

// applyTierPolicy normalizes tier and expiry fields for a fact and returns the normalized fact.
func applyTierPolicy(now time.Time, conf Config, fact MemoryFact) MemoryFact {
	normalized := fact
	switch normalized.Tier {
	case memoryTierL0:
		normalized.ExpiresAt = ""
	case memoryTierL1:
		normalized.ExpiresAt = expirationAt(now, conf.L1RetentionDays).Format(time.RFC3339)
	case memoryTierL2:
		normalized.ExpiresAt = expirationAt(now, conf.L2RetentionDays).Format(time.RFC3339)
	default:
		normalized.Tier = memoryTierL2
		normalized.ExpiresAt = expirationAt(now, conf.L2RetentionDays).Format(time.RFC3339)
	}

	return normalized
}

// expirationAt returns the UTC start time after retentionDays and is used as record expiry.
func expirationAt(now time.Time, retentionDays int) time.Time {
	if retentionDays <= 0 {
		return time.Time{}
	}

	start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return start.AddDate(0, 0, retentionDays)
}

// isFactExpired reports whether a fact is expired at now and returns true only when expiry is set and passed.
func isFactExpired(now time.Time, fact MemoryFact) bool {
	if strings.TrimSpace(fact.ExpiresAt) == "" {
		return false
	}

	expiresAt, err := time.Parse(time.RFC3339, fact.ExpiresAt)
	if err != nil {
		return false
	}

	return !expiresAt.After(now.UTC())
}

// marshalJSONL marshals known record slices into JSONL and returns a trailing-newline body.
func marshalJSONL(v any) (string, error) {
	switch records := v.(type) {
	case []LogEvent:
		return marshalLogEvents(records)
	case []MemoryFact:
		return marshalFacts(records)
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			return "", errors.Wrap(err, "marshal generic")
		}
		if string(buf) == "null" {
			return "", nil
		}
		return string(buf) + "\n", nil
	}
}

// marshalLogEvents marshals log events into JSONL and returns serialized content.
func marshalLogEvents(events []LogEvent) (string, error) {
	if len(events) == 0 {
		return "", nil
	}

	lines := make([]string, 0, len(events))
	for _, event := range events {
		buf, err := json.Marshal(event)
		if err != nil {
			return "", errors.Wrap(err, "marshal event")
		}
		lines = append(lines, string(buf))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// marshalFacts marshals memory facts into JSONL and returns serialized content.
func marshalFacts(facts []MemoryFact) (string, error) {
	if len(facts) == 0 {
		return "", nil
	}

	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		buf, err := json.Marshal(fact)
		if err != nil {
			return "", errors.Wrap(err, "marshal fact")
		}
		lines = append(lines, string(buf))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// parseJSONLLines splits body text and returns non-empty trimmed lines.
func parseJSONLLines(body string) []string {
	lines := strings.Split(body, "\n")
	records := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		records = append(records, line)
	}

	return records
}

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

// contains reports whether a string list contains value and returns true when found.
func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
}

// compressZstd compresses text content into zstd payload and returns encoded bytes.
func compressZstd(body string) ([]byte, error) {
	var buf bytes.Buffer
	encoder, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, errors.Wrap(err, "create zstd writer")
	}
	if _, err = encoder.Write([]byte(body)); err != nil {
		_ = encoder.Close()
		return nil, errors.Wrap(err, "write zstd body")
	}
	if err = encoder.Close(); err != nil {
		return nil, errors.Wrap(err, "close zstd writer")
	}

	return buf.Bytes(), nil
}

// wordCount counts words in the input text and returns the count.
func wordCount(text string) int {
	return len(strings.Fields(strings.TrimSpace(text)))
}
