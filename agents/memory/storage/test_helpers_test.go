package storage_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
	mcpstorage "github.com/Laisky/go-utils/v6/agents/memory/storage/mcp"
)

const (
	searchWaitTimeout  = 12 * time.Second
	searchPollInterval = 500 * time.Millisecond
)

// storageEngineFixture describes one storage engine instance factory used by behavior and e2e suites.
type storageEngineFixture struct {
	Name    string
	Project string
	Build   func(t *testing.T) (memorystorage.Engine, func())
}

// mcpEnvConfig stores MCP credentials loaded from environment variables.
type mcpEnvConfig struct {
	Endpoint string
	APIKey   string
	Project  string
}

// loadStorageEngineFixtures returns test fixtures with local engine by default and optional MCP engine.
//
// Parameters:
//   - t: The active test instance.
//
// Returns:
//   - []storageEngineFixture: Available engine fixtures for this test run.
func loadStorageEngineFixtures(t *testing.T) []storageEngineFixture {
	t.Helper()

	fixtures := []storageEngineFixture{
		{
			Name:    "local",
			Project: "memory-storage-tests-local",
			Build: func(t *testing.T) (memorystorage.Engine, func()) {
				t.Helper()

				rootDir := t.TempDir()
				engine, err := local.NewEngine(local.Config{RootDir: rootDir})
				require.NoError(t, err)

				cleanup := func() {
					require.NoError(t, engine.Close())
				}
				return engine, cleanup
			},
		},
	}

	if mcpConf, ok := loadMCPEnvConfig(); ok {
		fixtures = append(fixtures, storageEngineFixture{
			Name:    "mcp",
			Project: mcpConf.Project,
			Build: func(t *testing.T) (memorystorage.Engine, func()) {
				t.Helper()

				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()

				engine, err := mcpstorage.NewEngine(ctx, mcpstorage.Config{
					Endpoint: mcpConf.Endpoint,
					APIKey:   mcpConf.APIKey,
				})
				require.NoError(t, err)

				return engine, func() {}
			},
		})
	}

	return fixtures
}

// loadMCPEnvConfig loads MCP env vars and reports whether the MCP fixture should be enabled.
//
// Parameters:
//   - none.
//
// Returns:
//   - mcpEnvConfig: The parsed MCP config values.
//   - bool: True when all required MCP environment variables are available.
func loadMCPEnvConfig() (mcpEnvConfig, bool) {
	endpoint := firstNonEmptyEnv("MEMORY_MCP_ENDPOINT", "MCP_ENDPOINT")
	apiKey := firstNonEmptyEnv("MEMORY_MCP_API_KEY", "MCP_API_KEY")
	project := firstNonEmptyEnv("MEMORY_PROJECT", "MCP_PROJECT")
	if endpoint == "" || apiKey == "" || project == "" {
		return mcpEnvConfig{}, false
	}

	return mcpEnvConfig{
		Endpoint: endpoint,
		APIKey:   apiKey,
		Project:  project,
	}, true
}

// firstNonEmptyEnv returns the first non-empty environment variable value.
//
// Parameters:
//   - keys: Candidate environment variable names in lookup order.
//
// Returns:
//   - string: The first non-empty environment variable value.
func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}

	return ""
}

// waitForSearchHits waits until a search query returns at least one chunk.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation.
//   - engine: Storage engine used for searching.
//   - project: Project namespace used by the query.
//   - query: Search text query.
//   - pathPrefix: Prefix path limiting the query scope.
//   - limit: Maximum chunk count requested per poll.
//
// Returns:
//   - []memorystorage.FileChunk: Search chunks once indexed and available.
//   - error: Non-nil when timeout occurs, context is canceled, or storage returns a permanent error.
func waitForSearchHits(
	ctx context.Context,
	engine memorystorage.Engine,
	project, query, pathPrefix string,
	limit int,
) ([]memorystorage.FileChunk, error) {
	deadline := time.Now().Add(searchWaitTimeout)
	for time.Now().Before(deadline) {
		chunks, err := engine.Search(ctx, project, query, pathPrefix, limit)
		if err == nil && len(chunks) > 0 {
			return chunks, nil
		}
		if err != nil && isSearchBackendDisabled(err) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context done while waiting search hits")
		case <-time.After(searchPollInterval):
		}
	}

	return nil, errors.Errorf("search results not ready before timeout")
}

// isSearchBackendDisabled reports whether MCP search backend is unavailable.
//
// Parameters:
//   - err: Search error returned by storage engine.
//
// Returns:
//   - bool: True when the error indicates MCP search backend unavailability.
func isSearchBackendDisabled(err error) bool {
	var toolErr *files.ToolError
	if errors.As(err, &toolErr) && toolErr.Code == files.ErrorCodeSearchBackendError {
		return true
	}

	return false
}

// containsFileInfoPath reports whether entries include one exact path.
//
// Parameters:
//   - entries: File entries returned by list operations.
//   - targetPath: Exact path expected to exist.
//
// Returns:
//   - bool: True when targetPath exists in entries.
func containsFileInfoPath(entries []memorystorage.FileInfo, targetPath string) bool {
	for _, entry := range entries {
		if entry.Path == targetPath {
			return true
		}
	}

	return false
}
