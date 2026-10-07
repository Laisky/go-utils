package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNewEngineUsesConfiguredLLMHeuristicClient verifies that an engine built
// from LLMAPIBase and LLMAPIKey actually consults the LLM-backed heuristic
// client during AfterTurn. A shadowed variable in NewEngine previously built
// the client but left the engine's heuristic nil, silently disabling the
// configured model-assisted fact extraction.
func TestNewEngineUsesConfiguredLLMHeuristicClient(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"resp_x","output":[{"type":"function_call",` +
			`"name":"extract_and_merge_memories",` +
			`"arguments":"{\"updated_facts\":[],\"deleted_fact_ids\":[]}"}]}`))
	}))
	defer server.Close()

	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	engine, err := NewEngine(newMemoryStorageMock(), Config{
		LLMAPIBase:           server.URL,
		LLMAPIKey:            "synthetic-key",
		LLMAllowInsecureHTTP: true,
		TimeNow:              func() time.Time { return now },
	})
	require.NoError(t, err)

	beforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "wiring-session",
		TurnID:    "turn-1",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Bob."}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:    "demo",
		SessionID:  "wiring-session",
		TurnID:     "turn-1",
		InputItems: beforeOut.InputItems,
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "Hi Bob."}},
		}},
	})
	require.NoError(t, err)
	require.Positive(t, calls.Load(), "configured LLM heuristic client was never called")
}
