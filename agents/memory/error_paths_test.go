package memory

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// storageStub is a configurable files.Storage stub for targeted error-path tests.
type storageStub struct {
	readFn   func(ctx context.Context, project, path string, offset, length int64) (string, error)
	writeFn  func(ctx context.Context, project, path, content string, mode files.WriteMode, offset int64) error
	statFn   func(ctx context.Context, project, path string) (files.FileInfo, error)
	listFn   func(ctx context.Context, project, path string, depth, limit int) ([]files.FileInfo, bool, error)
	searchFn func(ctx context.Context, project, query, pathPrefix string, limit int) ([]files.FileChunk, error)
	deleteFn func(ctx context.Context, project, path string, recursive bool) error
}

// Read reads a file according to stub behavior and returns content or error.
func (stub *storageStub) Read(ctx context.Context, project, path string, offset, length int64) (string, error) {
	if stub.readFn != nil {
		return stub.readFn(ctx, project, path, offset, length)
	}
	return "", nil
}

// Write writes a file according to stub behavior and returns write status.
func (stub *storageStub) Write(ctx context.Context, project, path, content string, mode files.WriteMode, offset int64) error {
	if stub.writeFn != nil {
		return stub.writeFn(ctx, project, path, content, mode, offset)
	}
	return nil
}

// Stat returns metadata according to stub behavior.
func (stub *storageStub) Stat(ctx context.Context, project, path string) (files.FileInfo, error) {
	if stub.statFn != nil {
		return stub.statFn(ctx, project, path)
	}
	return files.FileInfo{Path: path, Exists: false, Type: files.FileTypeUnknown}, nil
}

// List returns directory entries according to stub behavior.
func (stub *storageStub) List(ctx context.Context, project, path string, depth, limit int) ([]files.FileInfo, bool, error) {
	if stub.listFn != nil {
		return stub.listFn(ctx, project, path, depth, limit)
	}
	return nil, false, nil
}

// Search returns search hits according to stub behavior.
func (stub *storageStub) Search(ctx context.Context, project, query, pathPrefix string, limit int) ([]files.FileChunk, error) {
	if stub.searchFn != nil {
		return stub.searchFn(ctx, project, query, pathPrefix, limit)
	}
	return nil, nil
}

// Delete deletes a path according to stub behavior.
func (stub *storageStub) Delete(ctx context.Context, project, path string, recursive bool) error {
	if stub.deleteFn != nil {
		return stub.deleteFn(ctx, project, path, recursive)
	}
	return nil
}

// TestPublicAPIsErrorPaths verifies public API error handling on storage failures.
func TestPublicAPIsErrorPaths(t *testing.T) {
	stub := &storageStub{
		statFn: func(_ context.Context, _, _ string) (files.FileInfo, error) {
			return files.FileInfo{}, errors.Errorf("stat failed")
		},
		listFn: func(_ context.Context, _, _ string, _, _ int) ([]files.FileInfo, bool, error) {
			return nil, false, errors.Errorf("list failed")
		},
	}

	engine, err := NewEngine(stub, Config{TimeNow: func() time.Time { return time.Unix(0, 0).UTC() }})
	require.NoError(t, err)

	_, err = engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s1",
		TurnID:       "t1",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "hello"}}}},
	})
	require.Error(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "s1",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: "hello",
			}},
		}},
	})
	require.Error(t, err)

	err = engine.RunMaintenance(context.Background(), "demo", "s1")
	require.Error(t, err)

	_, err = engine.ListDirWithAbstract(context.Background(), "demo", "s1", "", 1, 10)
	require.Error(t, err)
}

// TestInternalErrorPaths verifies internal helper error handling branches.
func TestInternalErrorPaths(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)

	err = engine.appendJSONL(context.Background(), "demo", "/x.jsonl", make(chan int))
	require.Error(t, err)

	err = storage.Write(context.Background(), "demo", "/meta/bad.json", "{not-json}", files.WriteModeTruncate, 0)
	require.NoError(t, err)
	_, err = engine.loadMetaFile(context.Background(), "demo", "/meta/bad.json")
	require.Error(t, err)
}

// TestWriteMetaErrorBranches verifies writeMeta propagates first and second write failures.
func TestWriteMetaErrorBranches(t *testing.T) {
	meta := MemoryMeta{Version: 1}
	writeCount := 0
	stub := &storageStub{
		writeFn: func(_ context.Context, _, _ string, _ string, _ files.WriteMode, _ int64) error {
			writeCount++
			if writeCount == 1 {
				return errors.Errorf("first write failed")
			}
			return nil
		},
	}
	engine, err := NewEngine(stub, Config{})
	require.NoError(t, err)
	err = engine.writeMeta(context.Background(), "demo", "s", meta)
	require.Error(t, err)

	writeCount = 0
	stub.writeFn = func(_ context.Context, _, _ string, _ string, _ files.WriteMode, _ int64) error {
		writeCount++
		if writeCount == 2 {
			return errors.Errorf("second write failed")
		}
		return nil
	}
	err = engine.writeMeta(context.Background(), "demo", "s", meta)
	require.Error(t, err)
}

// TestLoadJSONLErrorBranches verifies loadJSONL returns wrapped errors from stat and read failures.
func TestLoadJSONLErrorBranches(t *testing.T) {
	engineStatErr, err := NewEngine(&storageStub{
		statFn: func(_ context.Context, _, _ string) (files.FileInfo, error) {
			return files.FileInfo{}, errors.Errorf("stat err")
		},
	}, Config{})
	require.NoError(t, err)

	_, err = engineStatErr.loadJSONL(context.Background(), "demo", "/x")
	require.Error(t, err)

	engineReadErr, err := NewEngine(&storageStub{
		statFn: func(_ context.Context, _, path string) (files.FileInfo, error) {
			return files.FileInfo{Path: path, Exists: true, Type: files.FileTypeFile}, nil
		},
		readFn: func(_ context.Context, _, _ string, _, _ int64) (string, error) {
			return "", errors.Errorf("read err")
		},
	}, Config{})
	require.NoError(t, err)

	_, err = engineReadErr.loadJSONL(context.Background(), "demo", "/x")
	require.Error(t, err)
}

// TestMaintenanceValidationBranches verifies maintenance API validation branches.
func TestMaintenanceValidationBranches(t *testing.T) {
	engine, err := NewEngine(newMemoryStorageMock(), Config{})
	require.NoError(t, err)

	err = engine.RunMaintenance(context.Background(), "", "s")
	require.Error(t, err)
	err = engine.RunMaintenance(context.Background(), "demo", "")
	require.Error(t, err)

	_, err = engine.ListDirWithAbstract(context.Background(), "", "s", "", 0, 0)
	require.Error(t, err)
	_, err = engine.ListDirWithAbstract(context.Background(), "demo", "", "", 0, 0)
	require.Error(t, err)
}
