package memory

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// memoryStorageMock is an in-memory implementation of files.Storage for tests.
type memoryStorageMock struct {
	mu    sync.Mutex
	files map[string]string
}

// newMemoryStorageMock creates in-memory storage.
func newMemoryStorageMock() *memoryStorageMock {
	return &memoryStorageMock{files: make(map[string]string)}
}

// key builds namespaced key by project and path.
func (storage *memoryStorageMock) key(project, path string) string {
	return project + ":" + path
}

// Read reads file content from in-memory map.
func (storage *memoryStorageMock) Read(_ context.Context, project, path string, offset, length int64) (string, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	content, ok := storage.files[storage.key(project, path)]
	if !ok {
		return "", nil
	}

	if offset < 0 || offset > int64(len(content)) {
		return "", errors.Errorf("invalid offset")
	}
	content = content[offset:]
	if length >= 0 && length < int64(len(content)) {
		content = content[:length]
	}

	return content, nil
}

// Write writes content into in-memory map.
func (storage *memoryStorageMock) Write(_ context.Context, project, path, content string, mode files.WriteMode, offset int64) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	key := storage.key(project, path)
	current := storage.files[key]

	switch mode {
	case files.WriteModeAppend:
		storage.files[key] = current + content
	case files.WriteModeTruncate:
		storage.files[key] = content
	case files.WriteModeOverwrite:
		if offset < 0 || offset > int64(len(current)) {
			return errors.Errorf("invalid offset")
		}
		head := current[:offset]
		tail := ""
		if int(offset)+len(content) < len(current) {
			tail = current[int(offset)+len(content):]
		}
		storage.files[key] = head + content + tail
	default:
		return errors.Errorf("unsupported write mode")
	}

	return nil
}

// Stat checks metadata for in-memory path.
func (storage *memoryStorageMock) Stat(_ context.Context, project, path string) (files.FileInfo, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	_, ok := storage.files[storage.key(project, path)]
	if !ok {
		return files.FileInfo{Path: path, Exists: false, Type: files.FileTypeUnknown}, nil
	}

	return files.FileInfo{Path: path, Exists: true, Type: files.FileTypeFile, SizeBytes: int64(len(storage.files[storage.key(project, path)]))}, nil
}

// List lists all entries under a prefix.
func (storage *memoryStorageMock) List(_ context.Context, project, path string, depth, limit int) ([]files.FileInfo, bool, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	entries := make([]files.FileInfo, 0)
	prefix := storage.key(project, path)
	for key, content := range storage.files {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, files.FileInfo{Path: strings.TrimPrefix(key, project+":"), Exists: true, Type: files.FileTypeFile, SizeBytes: int64(len(content))})
		}
	}

	return entries, false, nil
}

// Search returns chunks containing query by naive substring match.
func (storage *memoryStorageMock) Search(_ context.Context, project, query, pathPrefix string, limit int) ([]files.FileChunk, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	chunks := make([]files.FileChunk, 0)
	for key, content := range storage.files {
		if !strings.HasPrefix(key, project+":"+pathPrefix) {
			continue
		}
		idx := strings.Index(strings.ToLower(content), strings.ToLower(query))
		if idx < 0 {
			continue
		}

		chunks = append(chunks, files.FileChunk{
			FilePath:   strings.TrimPrefix(key, project+":"),
			StartBytes: int64(idx),
			EndBytes:   int64(idx + len(query)),
			Content:    content,
			Score:      0.9,
		})
		if len(chunks) >= limit {
			break
		}
	}

	return chunks, nil
}

// Delete deletes one file from in-memory map.
func (storage *memoryStorageMock) Delete(_ context.Context, project, path string, _ bool) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	delete(storage.files, storage.key(project, path))
	return nil
}

// TestAfterTurnIdempotent verifies duplicated turn writes do not duplicate records.
func TestAfterTurnIdempotent(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: func() time.Time { return time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC) }})
	require.NoError(t, err)

	in := AfterTurnInput{
		Project:   "demo",
		SessionID: "s1",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "My name is Alice"}},
		}},
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "Hi Alice"}},
		}},
	}

	err = engine.AfterTurn(context.Background(), in)
	require.NoError(t, err)
	err = engine.AfterTurn(context.Background(), in)
	require.NoError(t, err)

	logBody, err := mockStorage.Read(context.Background(), "demo", "/memory/s1/log.jsonl", 0, -1)
	require.NoError(t, err)
	require.Equal(t, 2, len(nonEmptyLines(logBody)))
}

// TestBeforeTurnRecall verifies facts and history are recalled into input items.
func TestBeforeTurnRecall(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{TimeNow: time.Now})
	require.NoError(t, err)

	err = engine.AfterTurn(context.Background(), AfterTurnInput{
		Project:   "demo",
		SessionID: "s2",
		TurnID:    "t1",
		InputItems: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "I prefer concise answers"}},
		}},
		OutputItems: []ResponseItem{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponseContentPart{{Type: "output_text", Text: "Noted"}},
		}},
	})
	require.NoError(t, err)

	out, err := engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s2",
		TurnID:       "t2",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "help me"}}}},
		MaxInputTok:  120000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.InputItems)
	require.NotEmpty(t, out.RecallFactIDs)

	foundMemoryBlock := false
	for _, item := range out.InputItems {
		if item.Role == "developer" && len(item.Content) > 0 && strings.Contains(item.Content[0].Text, "Memory recall") {
			foundMemoryBlock = true
			break
		}
	}
	require.True(t, foundMemoryBlock)
}

// TestBeforeTurnCompaction verifies context compaction triggers when token budget is tight.
func TestBeforeTurnCompaction(t *testing.T) {
	mockStorage := newMemoryStorageMock()
	engine, err := NewEngine(mockStorage, Config{
		RecentContextItems: 2,
		CompactThreshold:   0.8,
		TimeNow:            func() time.Time { return time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)

	for idx := 0; idx < 6; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "demo",
			SessionID: "s3",
			TurnID:    "t" + string(rune('a'+idx)),
			InputItems: []ResponseItem{{
				Type:    "message",
				Role:    "user",
				Content: []ResponseContentPart{{Type: "input_text", Text: "This is a long sentence to force compaction and exceed token budget."}},
			}},
		})
		require.NoError(t, err)
	}

	_, err = engine.BeforeTurn(context.Background(), BeforeTurnInput{
		Project:      "demo",
		SessionID:    "s3",
		TurnID:       "t-final",
		CurrentInput: []ResponseItem{{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: "continue"}}}},
		MaxInputTok:  10,
	})
	require.NoError(t, err)

	ctxBody, err := mockStorage.Read(context.Background(), "demo", "/memory/s3/context.jsonl", 0, -1)
	require.NoError(t, err)
	require.Contains(t, ctxBody, "\"type\":\"compact\"")
}

// nonEmptyLines splits text and returns non-empty lines.
func nonEmptyLines(body string) []string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}
