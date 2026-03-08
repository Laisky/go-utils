package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAIResponsesClientExtractAndMergeFacts verifies tool-call parsing and fact normalization.
func TestOpenAIResponsesClientExtractAndMergeFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/v1/responses", request.URL.Path)
		require.NotEmpty(t, request.Header.Get("Authorization"))

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": "resp_x",
			"output": [{
				"type": "function_call",
				"name": "extract_and_merge_memories",
				"arguments": "{\"updated_facts\":[{\"fact_id\":\"user_name\",\"key\":\"name\",\"value\":\"alice\",\"tier\":\"L0\",\"confidence\":0.93},{\"fact_id\":\"today_task\",\"key\":\"today_task\",\"value\":\"finish tests\",\"tier\":\"L1\",\"confidence\":0.81}],\"deleted_fact_ids\":[] }"
			}]
		}`))
	}))
	defer server.Close()

	client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4.1",
	})
	require.NoError(t, err)

	result, err := client.ExtractAndMergeFacts(context.Background(), HeuristicFactInput{
		TurnID:     "turn-1",
		NowRFC3339: "2026-02-20T00:00:00Z",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "Please remember I am Alice and I need finish tests today.",
			}},
		}},
	})
	require.NoError(t, err)
	require.Len(t, result.UpdatedFacts, 2)
	require.Equal(t, "fact_upsert", result.UpdatedFacts[0].Type)
	require.NotEmpty(t, result.UpdatedFacts[0].ID)
	require.Equal(t, "2026-02-20T00:00:00Z", result.UpdatedFacts[0].TS)
	require.Empty(t, result.DeletedFactIDs)
}

// TestExtractHeuristicToolOutputNested verifies nested function payload argument extraction.
func TestExtractHeuristicToolOutputNested(t *testing.T) {
	payload := map[string]any{
		"output": []any{
			map[string]any{
				"type": "reasoning",
			},
			map[string]any{
				"type": "tool_call",
				"function": map[string]any{
					"name": "extract_and_merge_memories",
					"arguments": map[string]any{
						"updated_facts": []map[string]any{{
							"fact_id": "user_preference",
							"key":     "preference",
							"value":   "concise",
							"tier":    "L0",
						}},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	output, err := extractHeuristicToolOutput(body)
	require.NoError(t, err)
	require.Len(t, output.UpdatedFacts, 1)
	require.Equal(t, "user_preference", output.UpdatedFacts[0].FactID)
}

// TestNormalizeResponsesURL verifies responses endpoint normalization behavior.
func TestNormalizeResponsesURL(t *testing.T) {
	require.Equal(t, "https://oneapi.local/v1/responses", normalizeResponsesURL("https://oneapi.local"))
	require.Equal(t, "https://oneapi.local/v1/responses", normalizeResponsesURL("https://oneapi.local/"))
	require.Equal(t, "https://oneapi.local/v1/responses", normalizeResponsesURL("https://oneapi.local/v1/responses"))
	require.Equal(t, "https://oneapi.local/v1/responses", normalizeResponsesURL("oneapi.local"))
	require.Equal(t, "https://oneapi.local:8080/v1/responses", normalizeResponsesURL("oneapi.local:8080"))
	require.Equal(t, "https://oneapi.local/openai/v1/responses", normalizeResponsesURL("https://oneapi.local/openai"))
	require.Equal(t, "https://oneapi.local/openai/v1/responses", normalizeResponsesURL("https://oneapi.local/openai/v1"))
	require.Equal(t, "https://oneapi.local/openai/v1/responses", normalizeResponsesURL("https://oneapi.local/openai/v1/responses"))
}

// TestMergeFactCandidates verifies heuristic candidates override rule candidates by fact key.
func TestMergeFactCandidates(t *testing.T) {
	ruleFacts := []MemoryFact{{
		FactID: "user_preference",
		Key:    "preference",
		Value:  "short",
		Tier:   memoryTierL2,
	}}
	heuristicFacts := []MemoryFact{{
		FactID: "user_preference",
		Key:    "preference",
		Value:  "concise answers",
		Tier:   memoryTierL0,
	}}

	merged := mergeFactCandidates(ruleFacts, heuristicFacts)
	require.Len(t, merged, 1)
	require.Equal(t, "concise answers", merged[0].Value)
	require.Equal(t, memoryTierL0, merged[0].Tier)
}
