package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMemoryEngineBehaviorRoundTrip validates end-to-end memory lifecycle behavior.
func TestMemoryEngineBehaviorRoundTrip(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		RecentContextItems: 5,
		RecallFactsLimit:   10,
		SearchLimit:        5,
		TimeNow:            func() time.Time { return time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)

	beforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "behavior-session",
		TurnID:    "turn-1",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Bob and I prefer concise answers"}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.Len(t, beforeOut.InputItems, 1)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:    "demo",
		SessionID:  "behavior-session",
		TurnID:     "turn-1",
		InputItems: beforeOut.InputItems,
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "Nice to meet you Bob."}},
		}},
	})
	require.NoError(t, err)

	nextBeforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "behavior-session",
		TurnID:    "turn-2",
		CurrentInput: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "Do you remember my preference?"}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, nextBeforeOut.RecallFactIDs)

	metaBody, err := mockStorage.Read(context.Background(), "demo", "/memory/behavior-session/meta.json", 0, -1)
	require.NoError(t, err)
	require.Contains(t, metaBody, "turn-1")
}
