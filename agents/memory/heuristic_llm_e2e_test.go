package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHeuristicLLMEndToEnd verifies that real Responses API can generate memory facts for AfterTurn.
//
// Required env vars:
// 1. MEMORY_LLM_API_BASE (or LLM_API_BASE or API_BASE)
// 2. MEMORY_LLM_API_KEY (or LLM_API_KEY or API_KEY)
//
// Optional env vars:
// 1. MEMORY_LLM_MODEL (defaults to gpt-4.1)
func TestHeuristicLLMEndToEnd(t *testing.T) {
	apiBase := firstNonEmptyEnv("MEMORY_LLM_API_BASE", "LLM_API_BASE", "API_BASE")
	apiKey := firstNonEmptyEnv("MEMORY_LLM_API_KEY", "LLM_API_KEY", "API_KEY")
	model := firstNonEmptyEnv("MEMORY_LLM_MODEL", "LLM_MODEL")
	if strings.TrimSpace(apiBase) == "" || strings.TrimSpace(apiKey) == "" {
		t.Skip("skip e2e: set MEMORY_LLM_API_BASE/MEMORY_LLM_API_KEY (or LLM_API_BASE/LLM_API_KEY/API_BASE/API_KEY)")
	}

	now := time.Now().UTC()
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{
		TimeNow:            time.Now,
		LLMAPIBase:         apiBase,
		LLMAPIKey:          apiKey,
		LLMModel:           model,
		LLMTimeout:         20 * time.Second,
		LLMMaxOutputTokens: 1000,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	err = engine.AfterTurn(ctx, AfterTurnInput{
		Project:   "demo",
		SessionID: "llm-e2e",
		TurnID:    "t-llm-1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "Please remember this for future turns: I am called Vega, I strongly prefer bullet-point answers, and this week I need to finish memory integration tasks.",
			}},
		}},
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "Understood.",
			}},
		}},
	})
	require.NoError(t, err)

	factsFound := false
	for _, tier := range []string{memoryTierL0, memoryTierL1, memoryTierL2} {
		body, readErr := storage.Read(ctx, "demo", tierFactsShardPath("llm-e2e", tier, now), 0, -1)
		if readErr != nil {
			continue
		}
		if len(parseJSONLLines(body)) > 0 {
			factsFound = true
			break
		}
	}
	require.True(t, factsFound)

	if os.Getenv("MEMORY_LLM_E2E_DEBUG") == "1" {
		_, _ = storage.Read(ctx, "demo", tierFactsShardPath("llm-e2e", memoryTierL0, now), 0, -1)
	}
}
