package storage_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestStorageEngineBehavior validates storage-interface behavior on local engine by default and MCP when configured.
func TestStorageEngineBehavior(t *testing.T) {
	fixtures := loadStorageEngineFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			engine, cleanup := fixture.Build(t)
			defer cleanup()

			runStorageBehaviorSuite(t, engine, fixture.Project, fixture.Name)
		})
	}
}

// runStorageBehaviorSuite verifies write/read/stat/list/search/delete semantics for one engine implementation.
//
// Parameters:
//   - t: The active test instance.
//   - engine: Storage engine under test.
//   - project: Project namespace used for all requests.
//   - fixtureName: Fixture display name used to generate unique paths.
//
// Returns:
//   - none.
func runStorageBehaviorSuite(t *testing.T, engine memorystorage.Engine, project, fixtureName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	basePath := fmt.Sprintf("/memory/storage-behavior/%s-%d", fixtureName, time.Now().UTC().UnixNano())
	defer func() {
		err := engine.Delete(context.Background(), project, basePath, true)
		require.NoError(t, err)
	}()

	alphaPath := basePath + "/alpha.txt"
	nestedDirPath := basePath + "/nested"
	nestedFilePath := nestedDirPath + "/beta.txt"

	err := engine.Write(ctx, project, alphaPath, "hello", memorystorage.WriteModeTruncate, 0)
	require.NoError(t, err)
	err = engine.Write(ctx, project, alphaPath, " world", memorystorage.WriteModeAppend, 0)
	require.NoError(t, err)

	alphaBody, err := engine.Read(ctx, project, alphaPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, "hello world", alphaBody)

	slicedBody, err := engine.Read(ctx, project, alphaPath, 6, 5)
	require.NoError(t, err)
	require.Equal(t, "world", slicedBody)

	err = engine.Write(ctx, project, alphaPath, "Go", memorystorage.WriteModeOverwrite, 6)
	require.NoError(t, err)

	alphaBody, err = engine.Read(ctx, project, alphaPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, "hello Gorld", alphaBody)

	err = engine.Write(
		ctx,
		project,
		nestedFilePath,
		"nested data with search-token-omega",
		memorystorage.WriteModeTruncate,
		0,
	)
	require.NoError(t, err)

	alphaInfo, err := engine.Stat(ctx, project, alphaPath)
	require.NoError(t, err)
	require.True(t, alphaInfo.Exists)
	require.Equal(t, memorystorage.FileTypeFile, alphaInfo.Type)
	require.NotEmpty(t, strings.TrimSpace(alphaInfo.UpdatedAt))

	nestedInfo, err := engine.Stat(ctx, project, nestedDirPath)
	require.NoError(t, err)
	require.True(t, nestedInfo.Exists)
	require.Equal(t, memorystorage.FileTypeDirectory, nestedInfo.Type)

	entries, hasMore, err := engine.List(ctx, project, basePath, 4, 128)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.True(t, containsFileInfoPath(entries, alphaPath))
	require.True(t, containsFileInfoPath(entries, nestedFilePath))

	chunks, err := waitForSearchHits(ctx, engine, project, "search-token-omega", basePath, 5)
	if isSearchBackendDisabled(err) {
		t.Skip("search backend is disabled for current MCP environment")
	}
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	searchMatched := false
	for _, chunk := range chunks {
		if chunk.FilePath == nestedFilePath {
			searchMatched = true
			break
		}
	}
	require.True(t, searchMatched)

	err = engine.Delete(ctx, project, alphaPath, false)
	require.NoError(t, err)

	deletedInfo, err := engine.Stat(ctx, project, alphaPath)
	require.NoError(t, err)
	require.False(t, deletedInfo.Exists)

	err = engine.Delete(ctx, project, basePath, true)
	require.NoError(t, err)

	baseInfo, err := engine.Stat(ctx, project, basePath)
	require.NoError(t, err)
	require.False(t, baseInfo.Exists)

	err = engine.Write(ctx, project, basePath+"/invalid.txt", "bad", memorystorage.WriteModeOverwrite, 999)
	require.Error(t, err)
	require.Contains(t, err.Error(), "offset")
}
