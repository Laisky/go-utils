package files

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

var projectRegex = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// MCPStorageConfig controls behavior of MCP-backed storage adapter.
type MCPStorageConfig struct {
	Caller       ToolCaller
	RetryDelays  []time.Duration
	DefaultDepth int
	DefaultLimit int
}

// MCPStorage implements Storage by calling MCP FileIO tools.
type MCPStorage struct {
	caller       ToolCaller
	retryDelays  []time.Duration
	defaultDepth int
	defaultLimit int
}

// NewMCPStorage creates a Storage adapter backed by MCP FileIO tools.
func NewMCPStorage(conf MCPStorageConfig) (*MCPStorage, error) {
	if conf.Caller == nil {
		return nil, errors.Errorf("caller is required")
	}

	retryDelays := conf.RetryDelays
	if len(retryDelays) == 0 {
		retryDelays = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second}
	}

	defaultDepth := conf.DefaultDepth
	if defaultDepth <= 0 {
		defaultDepth = 1
	}

	defaultLimit := conf.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = 50
	}

	return &MCPStorage{
		caller:       conf.Caller,
		retryDelays:  retryDelays,
		defaultDepth: defaultDepth,
		defaultLimit: defaultLimit,
	}, nil
}

// NewMCPStorageFromConfig creates MCP storage using direct endpoint/apikey config.
func NewMCPStorageFromConfig(ctx context.Context, endpoint, apiKey string) (*MCPStorage, error) {
	client, err := NewMCPClient(MCPClientConfig{
		Endpoint: endpoint,
		APIKey:   apiKey,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new mcp client")
	}

	storage, err := NewMCPStorage(MCPStorageConfig{Caller: client})
	if err != nil {
		return nil, errors.Wrap(err, "new mcp storage")
	}

	if _, err = storage.Stat(ctx, "bootstrap", ""); err != nil {
		var tErr *ToolError
		if errors.As(err, &tErr) {
			if tErr.Code == ErrorCodeInvalidPath || tErr.Code == ErrorCodeNotFound {
				return storage, nil
			}
		}
	}

	return storage, nil
}

// Read reads content from file path with optional byte range.
func (storage *MCPStorage) Read(ctx context.Context, project, path string, offset, length int64) (string, error) {
	if err := validateProject(project); err != nil {
		return "", errors.Wrap(err, "validate project")
	}
	if err := validatePath(path, false); err != nil {
		return "", errors.Wrap(err, "validate path")
	}

	var out struct {
		Content string `json:"content"`
	}
	if err := storage.callWithRetry(ctx, "file_read", map[string]any{
		"project": project,
		"path":    path,
		"offset":  offset,
		"length":  length,
	}, &out); err != nil {
		return "", errors.Wrap(err, "call file_read")
	}

	return out.Content, nil
}

// Write writes content to file using selected write mode and offset.
func (storage *MCPStorage) Write(ctx context.Context,
	project, path, content string, mode WriteMode, offset int64) error {
	if err := validateProject(project); err != nil {
		return errors.Wrap(err, "validate project")
	}
	if err := validatePath(path, false); err != nil {
		return errors.Wrap(err, "validate path")
	}
	if err := validateWriteMode(mode); err != nil {
		return errors.Wrap(err, "validate write mode")
	}

	if err := storage.callWithRetry(ctx, "file_write", map[string]any{
		"project":          project,
		"path":             path,
		"content":          content,
		"content_encoding": "utf-8",
		"mode":             string(mode),
		"offset":           offset,
	}, nil); err != nil {
		return errors.Wrap(err, "call file_write")
	}

	return nil
}

// Stat checks metadata and existence for target path.
func (storage *MCPStorage) Stat(ctx context.Context, project, path string) (FileInfo, error) {
	if err := validateProject(project); err != nil {
		return FileInfo{}, errors.Wrap(err, "validate project")
	}
	if err := validatePath(path, true); err != nil {
		return FileInfo{}, errors.Wrap(err, "validate path")
	}

	var out struct {
		Exists    bool   `json:"exists"`
		Type      string `json:"type"`
		Size      int64  `json:"size"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := storage.callWithRetry(ctx, "file_stat", map[string]any{
		"project": project,
		"path":    path,
	}, &out); err != nil {
		return FileInfo{}, errors.Wrap(err, "call file_stat")
	}

	return FileInfo{
		Path:      path,
		Exists:    out.Exists,
		Type:      FileType(strings.ToUpper(out.Type)),
		SizeBytes: out.Size,
		UpdatedAt: out.UpdatedAt,
	}, nil
}

// List lists file entries under path with depth and limit control.
func (storage *MCPStorage) List(ctx context.Context,
	project, path string, depth, limit int) (entries []FileInfo, hasMore bool, err error) {
	if err = validateProject(project); err != nil {
		return nil, false, errors.Wrap(err, "validate project")
	}
	if err = validatePath(path, true); err != nil {
		return nil, false, errors.Wrap(err, "validate path")
	}

	if depth < 0 {
		depth = storage.defaultDepth
	}
	if limit <= 0 {
		limit = storage.defaultLimit
	}

	var out struct {
		Entries []struct {
			Path      string `json:"path"`
			Type      string `json:"type"`
			Size      int64  `json:"size"`
			UpdatedAt string `json:"updated_at"`
		} `json:"entries"`
		HasMore bool `json:"has_more"`
	}
	if err = storage.callWithRetry(ctx, "file_list", map[string]any{
		"project": project,
		"path":    path,
		"depth":   depth,
		"limit":   limit,
	}, &out); err != nil {
		return nil, false, errors.Wrap(err, "call file_list")
	}

	infos := make([]FileInfo, 0, len(out.Entries))
	for _, entry := range out.Entries {
		infos = append(infos, FileInfo{
			Path:      entry.Path,
			Exists:    true,
			Type:      FileType(strings.ToUpper(entry.Type)),
			SizeBytes: entry.Size,
			UpdatedAt: entry.UpdatedAt,
		})
	}

	return infos, out.HasMore, nil
}

// Search searches indexed chunks under project with optional path prefix.
func (storage *MCPStorage) Search(ctx context.Context,
	project, query, pathPrefix string, limit int) ([]FileChunk, error) {
	if err := validateProject(project); err != nil {
		return nil, errors.Wrap(err, "validate project")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.Errorf("query is required")
	}
	if err := validatePath(pathPrefix, true); err != nil {
		return nil, errors.Wrap(err, "validate path prefix")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	var out struct {
		Chunks []struct {
			FilePath     string  `json:"file_path"`
			StartBytes   int64   `json:"file_seek_start_bytes"`
			EndBytes     int64   `json:"file_seek_end_bytes"`
			ChunkContent string  `json:"chunk_content"`
			Score        float64 `json:"score"`
		} `json:"chunks"`
	}
	if err := storage.callWithRetry(ctx, "file_search", map[string]any{
		"project":     project,
		"query":       query,
		"path_prefix": pathPrefix,
		"limit":       limit,
	}, &out); err != nil {
		return nil, errors.Wrap(err, "call file_search")
	}

	chunks := make([]FileChunk, 0, len(out.Chunks))
	for _, chunk := range out.Chunks {
		chunks = append(chunks, FileChunk{
			FilePath:   chunk.FilePath,
			StartBytes: chunk.StartBytes,
			EndBytes:   chunk.EndBytes,
			Content:    chunk.ChunkContent,
			Score:      chunk.Score,
		})
	}

	return chunks, nil
}

// Delete deletes file or directory subtree.
func (storage *MCPStorage) Delete(ctx context.Context, project, path string, recursive bool) error {
	if err := validateProject(project); err != nil {
		return errors.Wrap(err, "validate project")
	}
	if err := validatePath(path, false); err != nil {
		return errors.Wrap(err, "validate path")
	}

	if err := storage.callWithRetry(ctx, "file_delete", map[string]any{
		"project":   project,
		"path":      path,
		"recursive": recursive,
	}, nil); err != nil {
		return errors.Wrap(err, "call file_delete")
	}

	return nil
}

// callWithRetry retries retryable storage calls with exponential-like delay.
func (storage *MCPStorage) callWithRetry(ctx context.Context, toolName string, args any, out any) error {
	var err error
	for idx := 0; idx <= len(storage.retryDelays); idx++ {
		err = storage.caller.CallTool(ctx, toolName, args, out)
		if err == nil {
			return nil
		}

		var tErr *ToolError
		if !errors.As(err, &tErr) || !tErr.IsRetryable() || idx == len(storage.retryDelays) {
			return errors.Wrap(err, "call tool")
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context done before retry")
		case <-time.After(storage.retryDelays[idx]):
		}
	}

	return errors.Wrap(err, "call tool retries exhausted")
}

// validateProject validates project identifier against MCP spec.
func validateProject(project string) error {
	if !projectRegex.MatchString(project) {
		return errors.Errorf("invalid project `%s`", project)
	}

	return nil
}

// validatePath validates path according to MCP FileIO path constraints.
func validatePath(path string, allowRoot bool) error {
	if path == "" {
		if allowRoot {
			return nil
		}
		return errors.Errorf("path cannot be empty")
	}

	if path == "/" {
		if allowRoot {
			return nil
		}
		return errors.Errorf("root slash path is not allowed")
	}

	if !strings.HasPrefix(path, "/") {
		return errors.Errorf("path must start with `/`")
	}
	if strings.HasSuffix(path, "/") {
		return errors.Errorf("path cannot end with `/`")
	}
	if strings.Contains(path, "//") {
		return errors.Errorf("path cannot contain `//`")
	}
	if strings.Contains(path, "/./") || strings.HasPrefix(path, "/./") || strings.HasSuffix(path, "/.") {
		return errors.Errorf("path cannot contain `.` segment")
	}
	if strings.Contains(path, "/../") || strings.HasPrefix(path, "/../") || strings.HasSuffix(path, "/..") {
		return errors.Errorf("path cannot contain `..` segment")
	}
	if strings.ContainsAny(path, " \t\n\r") {
		return errors.Errorf("path cannot contain spaces or control chars")
	}
	if len(path) > 512 {
		return errors.Errorf("path too long")
	}

	return nil
}

// validateWriteMode validates available write modes.
func validateWriteMode(mode WriteMode) error {
	switch mode {
	case WriteModeAppend, WriteModeOverwrite, WriteModeTruncate:
		return nil
	default:
		return errors.Errorf("invalid write mode `%s`", mode)
	}
}
