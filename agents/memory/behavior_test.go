package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMemoryEngineBehaviorRoundTrip validates memory lifecycle with tier retention behavior.
func TestMemoryEngineBehaviorRoundTrip(t *testing.T) {
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		RecentContextItems: 5,
		RecallFactsLimit:   10,
		SearchLimit:        5,
		TimeNow:            func() time.Time { return now },
	})
	require.NoError(t, err)

	beforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "behavior-session",
		TurnID:    "turn-1",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "My name is Bob. Today I need complete design doc.",
			}},
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
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "Nice to meet you Bob.",
			}},
		}},
	})
	require.NoError(t, err)

	nextBeforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "behavior-session",
		TurnID:    "turn-2",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "Do you remember my profile and task?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.Contains(t, nextBeforeOut.RecallFactIDs, "user_name")
	require.Contains(t, nextBeforeOut.RecallFactIDs, "today_task")

	now = now.AddDate(0, 0, 2)
	err = engine.RunMaintenance(context.Background(), "demo", "behavior-session")
	require.NoError(t, err)

	thirdBeforeOut, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:   "demo",
		SessionID: "behavior-session",
		TurnID:    "turn-3",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "What do you remember now?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.Contains(t, thirdBeforeOut.RecallFactIDs, "user_name")
	require.NotContains(t, thirdBeforeOut.RecallFactIDs, "today_task")

	metaBody, err := mockStorage.Read(context.Background(), "demo", metaStatePath("behavior-session"), 0, -1)
	require.NoError(t, err)
	require.Contains(t, metaBody, "last_maintenance_at")
}
