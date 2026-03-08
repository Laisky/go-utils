package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

const (
	e2eWaitSearchTimeout = 12 * time.Second
	e2ePollInterval      = 500 * time.Millisecond
)

// TestMemorySDKEndToEndWithMCP verifies end-to-end behavior of MCP storage and memory engine.
//
// Required env vars:
// 1. MEMORY_MCP_ENDPOINT (or MCP_ENDPOINT)
// 2. MEMORY_MCP_API_KEY (or MCP_API_KEY)
// 3. MEMORY_PROJECT
//
// The test is skipped automatically when any required env var is not provided.
func TestMemorySDKEndToEndWithMCP(t *testing.T) {
	endpoint := firstNonEmptyEnv("MEMORY_MCP_ENDPOINT", "MCP_ENDPOINT")
	apiKey := firstNonEmptyEnv("MEMORY_MCP_API_KEY", "MCP_API_KEY")
	project := strings.TrimSpace(os.Getenv("MEMORY_PROJECT"))
	if endpoint == "" || apiKey == "" || project == "" {
		t.Skip("skip e2e: set MEMORY_MCP_ENDPOINT/MEMORY_MCP_API_KEY/MEMORY_PROJECT (or MCP_ENDPOINT/MCP_API_KEY)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := files.NewMCPClient(files.MCPClientConfig{
		Endpoint: endpoint,
		APIKey:   apiKey,
	})
	require.NoError(t, err)

	storage, err := files.NewMCPStorage(files.MCPStorageConfig{Caller: client})
	require.NoError(t, err)

	sessionID := fmt.Sprintf("e2e-%d", time.Now().UTC().UnixNano())
	basePath := sessionBasePath(sessionID)
	rawFilePath := basePath + "/raw.txt"

	defer func() {
		_ = storage.Delete(context.Background(), project, basePath, true)
	}()

	// Storage SDK: Write / Read / Stat / List / Search / Delete.
	err = storage.Write(ctx, project, rawFilePath, "hello from e2e\n", files.WriteModeAppend, 0)
	require.NoError(t, err)

	fileInfo, err := storage.Stat(ctx, project, rawFilePath)
	require.NoError(t, err)
	require.True(t, fileInfo.Exists)
	require.Equal(t, files.FileTypeFile, fileInfo.Type)

	body, err := storage.Read(ctx, project, rawFilePath, 0, -1)
	require.NoError(t, err)
	require.Contains(t, body, "hello from e2e")

	entries, hasMore, err := storage.List(ctx, project, basePath, 2, 50)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.True(t, containsPath(entries, rawFilePath))

	err = waitForSearchReady(ctx, storage, project, "hello from e2e", basePath)
	if err != nil {
		var toolErr *files.ToolError
		if errors.As(err, &toolErr) && toolErr.Code == files.ErrorCodeSearchBackendError {
			t.Skip("skip e2e search assertions: MCP search backend is disabled")
		}
		require.NoError(t, err)
	}

	err = storage.Delete(ctx, project, rawFilePath, false)
	require.NoError(t, err)

	deletedInfo, err := storage.Stat(ctx, project, rawFilePath)
	require.NoError(t, err)
	require.False(t, deletedInfo.Exists)

	// Memory SDK: BeforeTurn / AfterTurn lifecycle and idempotency path.
	engine, err := NewEngine(storage, Config{
		RecentContextItems: 30,
		RecallFactsLimit:   20,
		SearchLimit:        5,
		CompactThreshold:   0.8,
		TimeNow:            time.Now,
	})
	require.NoError(t, err)

	beforeOut, err := engine.BeforeTurn(ctx, BeforeTurnInput{
		Project:   project,
		SessionID: sessionID,
		UserID:    "e2e-user",
		TurnID:    "turn-1",
		ConversationItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "My name is E2EUser and I prefer concise answers. Today I need finish testing",
			}},
		}},
		CurrentInputStart: 0,
		CurrentInputCount: 1,
		MaxInputTok:       120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, beforeOut.InputItems)

	afterIn := AfterTurnInput{
		Project:           project,
		SessionID:         sessionID,
		UserID:            "e2e-user",
		TurnID:            "turn-1",
		ConversationItems: beforeOut.InputItems,
		CurrentInputStart: len(beforeOut.InputItems) - 1,
		CurrentInputCount: 1,
		OutputItems: []ResponseItem{{
			Type: "message",
			Role: "assistant",
			Content: []ResponseContentPart{{
				Type: "output_text",
				Text: "Noted. I will keep responses concise.",
			}},
		}},
	}

	err = engine.AfterTurn(ctx, afterIn)
	require.NoError(t, err)
	// Call again to verify idempotent processing by turn id.
	err = engine.AfterTurn(ctx, afterIn)
	require.NoError(t, err)

	recallOut, err := engine.BeforeTurn(ctx, BeforeTurnInput{
		Project:   project,
		SessionID: sessionID,
		UserID:    "e2e-user",
		TurnID:    "turn-2",
		ConversationItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "What is my preference?",
			}},
		}},
		CurrentInputStart: 0,
		CurrentInputCount: 1,
		MaxInputTok:       120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, recallOut.InputItems)
	require.NotEmpty(t, recallOut.RecallFactIDs)

	stateInfo, err := storage.Stat(ctx, project, metaStatePath(sessionID))
	require.NoError(t, err)
	require.True(t, stateInfo.Exists)

	policyInfo, err := storage.Stat(ctx, project, metaPolicyPath(sessionID))
	require.NoError(t, err)
	require.True(t, policyInfo.Exists)

	ctxInfo, err := storage.Stat(ctx, project, runtimeContextPath(sessionID))
	require.NoError(t, err)
	require.True(t, ctxInfo.Exists)

	rawEntries, _, err := storage.List(ctx, project, eventsRawRootPath(sessionID), 8, 100)
	require.NoError(t, err)
	require.True(t, containsJSONL(rawEntries))

	l0Entries, _, err := storage.List(ctx, project, tierRootPath(sessionID, memoryTierL0), 8, 100)
	require.NoError(t, err)
	require.True(t, containsJSONL(l0Entries))

	err = engine.RunMaintenance(ctx, project, sessionID)
	require.NoError(t, err)

	dirs, err := engine.ListDirWithAbstract(ctx, project, sessionID, "", 8, 200)
	require.NoError(t, err)
	require.NotEmpty(t, dirs)
}

// firstNonEmptyEnv returns the first non-empty environment variable value.
func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}

	return ""
}

// containsPath reports whether entries include target path.
func containsPath(entries []files.FileInfo, targetPath string) bool {
	for _, entry := range entries {
		if entry.Path == targetPath {
			return true
		}
	}

	return false
}

// containsJSONL reports whether entries include at least one jsonl file.
func containsJSONL(entries []files.FileInfo) bool {
	for _, entry := range entries {
		if entry.Type == files.FileTypeFile && strings.HasSuffix(entry.Path, ".jsonl") {
			return true
		}
	}

	return false
}

// waitForSearchReady waits until file_search can find the query or timeout.
func waitForSearchReady(ctx context.Context, storage files.Storage, project, query, pathPrefix string) error {
	deadline := time.Now().Add(e2eWaitSearchTimeout)
	for time.Now().Before(deadline) {
		chunks, err := storage.Search(ctx, project, query, pathPrefix, 5)
		if err == nil && len(chunks) > 0 {
			return nil
		}

		var toolErr *files.ToolError
		if err != nil && errors.As(err, &toolErr) && toolErr.Code == files.ErrorCodeSearchBackendError {
			return err
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context done while waiting search")
		case <-time.After(e2ePollInterval):
		}
	}

	return errors.Errorf("search result not ready before timeout")
}
