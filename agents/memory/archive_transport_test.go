package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// archiveJSONTransport executes the real client's serialized HTTP requests against
// a text-only FileIO fixture, including JSON serialization of the read response.
type archiveJSONTransport struct {
	storage *memoryStorageMock
	corrupt bool
}

// Do decodes an MCP request and returns a JSON-encoded FileIO response or an error.
func (transport *archiveJSONTransport) Do(req *http.Request) (*http.Response, error) {
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
			Args struct {
				Project   string          `json:"project"`
				Path      string          `json:"path"`
				Content   string          `json:"content"`
				Encoding  string          `json:"content_encoding"`
				Mode      files.WriteMode `json:"mode"`
				Offset    int64           `json:"offset"`
				Length    int64           `json:"length"`
				Depth     int             `json:"depth"`
				Limit     int             `json:"limit"`
				Recursive bool            `json:"recursive"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&request); err != nil {
		return nil, errors.Wrap(err, "decode fixture request")
	}
	args := request.Params.Args
	var result any = map[string]any{}
	var err error
	switch request.Method {
	case "initialize":
	case "tools/call":
		switch request.Params.Name {
		case "file_write":
			if args.Encoding != "utf-8" {
				return nil, errors.New("fixture only supports UTF-8")
			}
			err = transport.storage.Write(req.Context(), args.Project, args.Path, args.Content, args.Mode, args.Offset)
		case "file_read":
			var content string
			content, err = transport.storage.Read(req.Context(), args.Project, args.Path, args.Offset, args.Length)
			if transport.corrupt && strings.Contains(args.Path, "/archive/") {
				content = "truncated archive"
			}
			result = map[string]any{"content": content, "content_encoding": "utf-8"}
		case "file_delete":
			err = transport.storage.Delete(req.Context(), args.Project, args.Path, args.Recursive)
		case "file_stat":
			var info files.FileInfo
			info, err = transport.storage.Stat(req.Context(), args.Project, args.Path)
			result = map[string]any{"exists": info.Exists, "type": info.Type, "size": info.SizeBytes}
		case "file_list":
			var entries []files.FileInfo
			var more bool
			entries, more, err = transport.storage.List(req.Context(), args.Project, args.Path, args.Depth, args.Limit)
			items := make([]map[string]any, 0, len(entries))
			for _, info := range entries {
				items = append(items, map[string]any{"path": info.Path, "type": info.Type, "size": info.SizeBytes})
			}
			result = map[string]any{"entries": items, "has_more": more}
		default:
			return nil, errors.Errorf("unexpected fixture tool %s", request.Params.Name)
		}
	default:
		return nil, errors.Errorf("unexpected fixture method %s", request.Method)
	}
	if err != nil {
		return nil, errors.Wrap(err, "execute fixture operation")
	}
	encodedResult, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "marshal fixture result")
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": request.ID,
		"result": map[string]any{"content": []map[string]string{{"type": "text", "text": string(encodedResult)}}},
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal fixture response")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Mcp-Session-Id": []string{"archive-fixture"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

// newArchiveMCPStorage creates a real MCP client over an in-memory JSON transport.
func newArchiveMCPStorage(t *testing.T, transport *archiveJSONTransport) *files.MCPStorage {
	t.Helper()
	client, err := files.NewMCPClient(files.MCPClientConfig{
		Endpoint: "https://archive.test/mcp", APIKey: "synthetic-test-key", Client: transport,
	})
	require.NoError(t, err)
	storage, err := files.NewMCPStorage(files.MCPStorageConfig{Caller: client})
	require.NoError(t, err)
	return storage
}

// TestArchiveLegacyMCPRetainsRaw prevents deletion after binary zstd has crossed
// the actual lossy JSON/UTF-8 transport, rather than a pre-serialization mock.
func TestArchiveLegacyMCPRetainsRaw(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	backend := newMemoryStorageMock()
	storage := newArchiveMCPStorage(t, &archiveJSONTransport{storage: backend})
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)
	rawPath := rawLogShardPath("archive-test", now.AddDate(0, 0, -2))
	const raw = "{\"id\":\"original-event\"}\n"
	require.NoError(t, storage.Write(ctx, "demo", rawPath, raw, files.WriteModeTruncate, 0))
	err = engine.archiveRawShards(ctx, "demo", "archive-test")
	require.Error(t, err)
	retained, err := storage.Read(ctx, "demo", rawPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, raw, retained)
}
