package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// TestExtractFactsAndApplyTierPolicy verifies rule extraction and tier-expiry assignment.
func TestExtractFactsAndApplyTierPolicy(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	conf := Config{L1RetentionDays: 1, L2RetentionDays: 7}
	facts := extractFacts("turn-x", now.Format(time.RFC3339), []ResponseItem{{
		Type: "message",
		Role: "user",
		Content: []ResponseContentPart{{
			Type: "input_text",
			Text: "My name is Alice. I prefer concise style. I like coffee. Today I need finish tests.",
		}},
	}})
	require.GreaterOrEqual(t, len(facts), 4)

	byTier := map[string]int{}
	for _, fact := range facts {
		normalized := applyTierPolicy(now, conf, fact)
		byTier[normalized.Tier]++
		switch normalized.Tier {
		case memoryTierL0:
			require.Equal(t, "", normalized.ExpiresAt)
		case memoryTierL1, memoryTierL2:
			require.NotEmpty(t, normalized.ExpiresAt)
		}
	}
	require.GreaterOrEqual(t, byTier[memoryTierL0], 2)
	require.GreaterOrEqual(t, byTier[memoryTierL1], 1)

	unknownTier := applyTierPolicy(now, conf, MemoryFact{Tier: "UNKNOWN"})
	require.Equal(t, memoryTierL2, unknownTier.Tier)
	require.NotEmpty(t, unknownTier.ExpiresAt)
}

// TestMarshalParseAndSortFacts verifies JSONL helpers and recall ordering behavior.
func TestMarshalParseAndSortFacts(t *testing.T) {
	facts := []MemoryFact{
		{FactID: "a", TS: "2026-02-13T00:00:00Z", Confidence: 0.8},
		{FactID: "a", TS: "2026-02-14T00:00:00Z", Confidence: 0.9},
		{FactID: "b", TS: "2026-02-14T00:00:00Z", Confidence: 0.7},
	}

	body, err := marshalFacts(facts)
	require.NoError(t, err)
	records := parseJSONLLines(body)
	require.Len(t, records, 3)

	decoded := make([]MemoryFact, 0, len(records))
	for _, record := range records {
		var fact MemoryFact
		err = json.Unmarshal([]byte(record), &fact)
		require.NoError(t, err)
		decoded = append(decoded, fact)
	}

	decoded = rankFactsForRecall(
		time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		decoded,
		"",
	)
	unique := deduplicateFacts(decoded)
	require.Len(t, unique, 2)
	require.Equal(t, "a", unique[0].FactID)
}

// TestExpirationAndParseDate verifies expiration boundary and shard-date parsing behavior.
func TestExpirationAndParseDate(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 30, 0, 0, time.UTC)
	expiration := expirationAt(now, 1)
	require.Equal(t, "2026-02-15T00:00:00Z", expiration.Format(time.RFC3339))
	require.True(t, expirationAt(now, 0).IsZero())

	require.True(t, isFactExpired(now, MemoryFact{ExpiresAt: "2026-02-14T00:00:00Z"}))
	require.False(t, isFactExpired(now, MemoryFact{ExpiresAt: "2026-02-16T00:00:00Z"}))

	parsed, ok := parseRawShardDate("/memory/s/events/raw/2026/02/14/log-20260214.jsonl")
	require.True(t, ok)
	require.Equal(t, "2026-02-14", parsed.Format("2006-01-02"))

	_, ok = parseRawShardDate("/memory/s/events/raw/not-a-date/log.jsonl")
	require.False(t, ok)
}

// TestCompressionAndSummaries verifies zstd compression and summary text constraints.
func TestCompressionAndSummaries(t *testing.T) {
	compressed, err := compressZstd("hello memory")
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	decompressed, err := decoder.DecodeAll(compressed, nil)
	require.NoError(t, err)
	require.Equal(t, "hello memory", string(decompressed))

	abstract := buildDefaultAbstract("/memory/s/runtime")
	require.GreaterOrEqual(t, wordCount(abstract), 100)
	require.LessOrEqual(t, wordCount(abstract), 200)

	overview := buildDefaultOverview("/memory/s/runtime")
	require.Greater(t, len(overview), 10)
	require.LessOrEqual(t, wordCount(overview), 2000)
}

// TestValidationHelpers verifies before-turn and after-turn input validation behavior.
func TestValidationHelpers(t *testing.T) {
	err := validateBeforeTurnInput(BeforeTurnInput{})
	require.Error(t, err)
	require.True(t, IsValidationError(err))
	code, ok := ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeProjectRequired, code)

	err = validateBeforeTurnInput(BeforeTurnInput{Project: "demo"})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDRequired, code)

	err = validateBeforeTurnInput(BeforeTurnInput{Project: "demo", SessionID: "s"})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeTurnIDRequired, code)

	err = validateBeforeTurnInput(BeforeTurnInput{Project: "demo", SessionID: "s", TurnID: "t"})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeCurrentInputRequired, code)

	err = validateAfterTurnInput(AfterTurnInput{})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeProjectRequired, code)

	err = validateAfterTurnInput(AfterTurnInput{Project: "demo"})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDRequired, code)

	err = validateAfterTurnInput(AfterTurnInput{Project: "demo", SessionID: "s"})
	require.Error(t, err)
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeTurnIDRequired, code)

	err = validateBeforeTurnInput(BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s",
		TurnID:       "t",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "hi"}}}},
	})
	require.NoError(t, err)
}

// TestValidationErrorCodeFromWrappedError verifies validation error code survives engine-level wrapping.
func TestValidationErrorCodeFromWrappedError(t *testing.T) {
	engine, err := NewEngine(newMemoryStorageMock(), Config{})
	require.NoError(t, err)

	_, err = engine.BeforeTurn(context.Background(), BeforeTurnInput{})
	require.Error(t, err)
	require.True(t, IsValidationError(err))
	code, ok := ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeProjectRequired, code)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{Project: "demo", SessionID: "s"})
	require.Error(t, err)
	require.True(t, IsValidationError(err))
	code, ok = ValidationErrorCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeTurnIDRequired, code)
}

// TestValidateSessionID verifies session ID validation rejects path-traversal and
// control-character ids while accepting legitimate single-segment ids.
func TestValidateSessionID(t *testing.T) {
	rejected := []string{
		"",                      // empty
		"..",                    // parent traversal token
		".",                     // current-dir token
		"foo/bar",               // forward slash separator
		"foo/../bar",            // embedded traversal
		"a\\b",                  // backslash separator
		"a\x00b",                // NUL control char
		"a\nb",                  // newline control char
		"a/b",                   // separator
		string(rune(127)) + "x", // DEL control char
	}
	for _, sessionID := range rejected {
		err := validateSessionID(sessionID)
		require.Error(t, err, "expected rejection for %q", sessionID)
		require.True(t, IsValidationError(err), "expected validation error for %q", sessionID)
	}

	// Empty must map to the required code; other malformed ids to invalid.
	emptyCode, ok := ValidationErrorCodeFromError(validateSessionID(""))
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDRequired, emptyCode)

	invalidCode, ok := ValidationErrorCodeFromError(validateSessionID(".."))
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDInvalid, invalidCode)

	// Overlong ids are rejected as invalid.
	longCode, ok := ValidationErrorCodeFromError(validateSessionID(strings.Repeat("a", 129)))
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDInvalid, longCode)

	accepted := []string{"session-123", "abc_DEF.1", "s", "..foo", "foo.."}
	for _, sessionID := range accepted {
		require.NoError(t, validateSessionID(sessionID), "expected acceptance for %q", sessionID)
	}
}

// TestValidateInputRejectsTraversalSessionID verifies the public validate functions
// reject a traversal session ID end-to-end so cross-session escape cannot occur.
func TestValidateInputRejectsTraversalSessionID(t *testing.T) {
	beforeErr := validateBeforeTurnInput(BeforeTurnInput{
		Project:      "demo",
		SessionID:    "foo/../victimSession",
		TurnID:       "t",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "hi"}}}},
	})
	require.Error(t, beforeErr)
	beforeCode, ok := ValidationErrorCodeFromError(beforeErr)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDInvalid, beforeCode)

	afterErr := validateAfterTurnInput(AfterTurnInput{
		Project:   "demo",
		SessionID: "..",
		TurnID:    "t",
	})
	require.Error(t, afterErr)
	afterCode, ok := ValidationErrorCodeFromError(afterErr)
	require.True(t, ok)
	require.Equal(t, ValidationErrorCodeSessionIDInvalid, afterCode)
}

// TestMarshalJSONLVariants verifies JSONL marshaling across supported and generic record types.
func TestMarshalJSONLVariants(t *testing.T) {
	eventsBody, err := marshalJSONL([]LogEvent{{ID: "1", TS: "2026-02-14T00:00:00Z", Type: "input_item"}})
	require.NoError(t, err)
	require.Contains(t, eventsBody, "\"type\":\"input_item\"")

	genericBody, err := marshalJSONL(map[string]string{"k": "v"})
	require.NoError(t, err)
	require.Contains(t, genericBody, "\"k\":\"v\"")

	emptyBody, err := marshalJSONL(nil)
	require.NoError(t, err)
	require.Equal(t, "", emptyBody)
}
