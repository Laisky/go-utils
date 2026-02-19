package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/memory"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestStorageEngineEndToEnd validates end-to-end integration between memory engine and storage plugins.
func TestStorageEngineEndToEnd(t *testing.T) {
	fixtures := loadStorageEngineFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			engine, cleanup := fixture.Build(t)
			defer cleanup()

			runStorageE2ESuite(t, engine, fixture.Project, fixture.Name)
		})
	}
}

// runStorageE2ESuite executes a complete memory turn lifecycle on top of one storage engine implementation.
//
// Parameters:
//   - t: The active test instance.
//   - engine: Storage engine used by memory.NewEngine.
//   - project: Project namespace used for all operations.
//   - fixtureName: Fixture display name used to generate unique IDs.
//
// Returns:
//   - none.
func runStorageE2ESuite(t *testing.T, engine memorystorage.Engine, project, fixtureName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	sessionID := fmt.Sprintf("storage-e2e-%s-%d", fixtureName, time.Now().UTC().UnixNano())
	sessionBase := "/memory/" + sessionID
	defer func() {
		err := engine.Delete(context.Background(), project, sessionBase, true)
		require.NoError(t, err)
	}()

	memoryEngine, err := memory.NewEngine(engine, memory.Config{
		RecentContextItems: 30,
		RecallFactsLimit:   20,
		SearchLimit:        5,
		CompactThreshold:   0.8,
		TimeNow:            func() time.Time { return now },
	})
	require.NoError(t, err)

	firstBefore, err := memoryEngine.BeforeTurn(ctx, memory.BeforeTurnInput{
		Project:   project,
		SessionID: sessionID,
		UserID:    "storage-e2e-user",
		TurnID:    "turn-1",
		CurrentInput: []memory.ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "My name is E2EUser. Today I need finish testing. I prefer concise responses.",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, firstBefore.InputItems)

	err = memoryEngine.AfterTurn(ctx, memory.AfterTurnInput{
		Project:    project,
		SessionID:  sessionID,
		UserID:     "storage-e2e-user",
		TurnID:     "turn-1",
		InputItems: firstBefore.InputItems,
		OutputItems: []memory.ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []memory.ResponseContentPart{{
				Type: "output_text",
				Text: "Stored. I will respond concisely.",
			}},
		}},
	})
	require.NoError(t, err)

	recallBefore, err := memoryEngine.BeforeTurn(ctx, memory.BeforeTurnInput{
		Project:   project,
		SessionID: sessionID,
		UserID:    "storage-e2e-user",
		TurnID:    "turn-2",
		CurrentInput: []memory.ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "What do you remember?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.Contains(t, recallBefore.RecallFactIDs, "user_name")
	require.Contains(t, recallBefore.RecallFactIDs, "today_task")

	var mgmt memory.Management = memoryEngine
	now = now.AddDate(0, 0, 2)
	err = mgmt.RunMaintenance(ctx, project, sessionID)
	require.NoError(t, err)

	afterMaintenance, err := memoryEngine.BeforeTurn(ctx, memory.BeforeTurnInput{
		Project:   project,
		SessionID: sessionID,
		UserID:    "storage-e2e-user",
		TurnID:    "turn-3",
		CurrentInput: []memory.ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []memory.ResponseContentPart{{
				Type: "input_text",
				Text: "What is still remembered?",
			}},
		}},
		MaxInputTok: 120000,
	})
	require.NoError(t, err)
	require.Contains(t, afterMaintenance.RecallFactIDs, "user_name")
	require.NotContains(t, afterMaintenance.RecallFactIDs, "today_task")

	stateInfo, err := engine.Stat(ctx, project, sessionBase+"/meta/state.json")
	require.NoError(t, err)
	require.True(t, stateInfo.Exists)

	contextInfo, err := engine.Stat(ctx, project, sessionBase+"/runtime/context/current.jsonl")
	require.NoError(t, err)
	require.True(t, contextInfo.Exists)

	rawEntries, _, err := engine.List(ctx, project, sessionBase+"/events/raw", 8, 100)
	require.NoError(t, err)
	require.NotEmpty(t, rawEntries)

	l0Entries, _, err := engine.List(ctx, project, sessionBase+"/memory_tiers/L0", 8, 100)
	require.NoError(t, err)
	require.NotEmpty(t, l0Entries)

	dirs, err := mgmt.ListDirWithAbstract(ctx, project, sessionID, "", 8, 200)
	require.NoError(t, err)
	require.NotEmpty(t, dirs)
}
