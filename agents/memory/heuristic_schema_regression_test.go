package memory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMemoryHeuristicToolSpecWireFormat pins the exact JSON wire format of the
// heuristic function-tool schema sent to the Responses API, so refactoring the
// schema construction (the goconst cleanup) cannot change what the model sees.
func TestMemoryHeuristicToolSpecWireFormat(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(memoryHeuristicToolSpec())
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "function",
		"name": "extract_and_merge_memories",
		"description": "Extract key facts, classify memory tier, and merge with existing memory facts.",
		"parameters": {
			"type": "object",
			"properties": {
				"updated_facts": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"fact_id": {"type": "string"},
							"key": {"type": "string"},
							"value": {"type": "string"},
							"tier": {"type": "string", "enum": ["L0", "L1", "L2"]},
							"confidence": {"type": "number"}
						},
						"required": ["fact_id", "key", "value", "tier"]
					}
				},
				"deleted_fact_ids": {"type": "array", "items": {"type": "string"}},
				"classifier_notes": {"type": "string"}
			},
			"required": ["updated_facts"]
		}
	}`, string(encoded))
}

// TestResponseItemIdentityIsStable pins identity hashes derived from the JSON
// field names of an item and its content parts, and the deterministic insight
// ID, so replacing those literal keys with constants cannot silently change
// persisted identities and break de-duplication of existing memory data.
func TestResponseItemIdentityIsStable(t *testing.T) {
	t.Parallel()
	item := ResponseItem{
		Type: "message", Role: "user", CallID: "call-1", Output: "out",
		Content: []ResponseContentPart{{Type: "input_text", Text: "hello", ImageURL: "u", FileID: "f", Filename: "n"}},
	}
	require.Equal(t, "item-fb82720cd8d53fe6", responseItemIdentity(item))
	require.Equal(t, "insight-af6e991ee41eb334",
		buildInsightID("identity", "fact_stability", "2026-01-02T03:04:05Z", "summary"))
}
