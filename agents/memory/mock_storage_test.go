package memory

import (
	"context"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// memoryStorageMock is an in-memory implementation of files.Storage for tests.
type memoryStorageMock struct {
	mu    sync.Mutex
	files map[string]string
}

// newMemoryStorageMock creates in-memory storage for one test run.
func newMemoryStorageMock() *memoryStorageMock {
	return &memoryStorageMock{files: make(map[string]string)}
}

// key builds namespaced key by project and path.
func (storage *memoryStorageMock) key(project, filePath string) string {
	return project + ":" + filePath
}

// Read reads file content from in-memory storage and returns sliced content.
func (storage *memoryStorageMock) Read(_ context.Context, project, filePath string, offset, length int64) (string, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	content, ok := storage.files[storage.key(project, filePath)]
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

// Write writes content into in-memory storage using a selected write mode.
func (storage *memoryStorageMock) Write(_ context.Context, project, filePath, content string, mode files.WriteMode, offset int64) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	key := storage.key(project, filePath)
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

// Stat returns metadata for one path and detects both files and directories.
func (storage *memoryStorageMock) Stat(_ context.Context, project, filePath string) (files.FileInfo, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	if content, ok := storage.files[storage.key(project, filePath)]; ok {
		return files.FileInfo{Path: filePath, Exists: true, Type: files.FileTypeFile, SizeBytes: int64(len(content))}, nil
	}

	dirPrefix := storage.key(project, strings.TrimSuffix(filePath, "/")+"/")
	for key := range storage.files {
		if strings.HasPrefix(key, dirPrefix) {
			return files.FileInfo{Path: filePath, Exists: true, Type: files.FileTypeDirectory}, nil
		}
	}

	return files.FileInfo{Path: filePath, Exists: false, Type: files.FileTypeUnknown}, nil
}

// List lists files and inferred directories under a path prefix.
func (storage *memoryStorageMock) List(_ context.Context, project, filePath string, depth, limit int) ([]files.FileInfo, bool, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	if depth <= 0 {
		depth = 1
	}
	if limit <= 0 {
		limit = 1000
	}

	prefixPath := strings.TrimSuffix(filePath, "/")
	prefixKey := storage.key(project, prefixPath)
	dirSet := make(map[string]struct{})
	fileInfos := make([]files.FileInfo, 0)

	for key, content := range storage.files {
		if !strings.HasPrefix(key, prefixKey) {
			continue
		}
		actualPath := strings.TrimPrefix(key, project+":")
		if actualPath != prefixPath && !strings.HasPrefix(actualPath, prefixPath+"/") {
			continue
		}

		relative := strings.TrimPrefix(actualPath, prefixPath)
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			fileInfos = append(fileInfos, files.FileInfo{Path: actualPath, Exists: true, Type: files.FileTypeFile, SizeBytes: int64(len(content))})
			continue
		}

		if strings.Count(relative, "/")+1 > depth+1 {
			continue
		}

		fileInfos = append(fileInfos, files.FileInfo{Path: actualPath, Exists: true, Type: files.FileTypeFile, SizeBytes: int64(len(content))})

		parent := path.Dir(actualPath)
		for parent != "." && parent != "/" && strings.HasPrefix(parent, prefixPath) {
			dirSet[parent] = struct{}{}
			if parent == prefixPath {
				break
			}
			parent = path.Dir(parent)
		}
	}

	entries := make([]files.FileInfo, 0, len(fileInfos)+len(dirSet))
	for dir := range dirSet {
		entries = append(entries, files.FileInfo{Path: dir, Exists: true, Type: files.FileTypeDirectory})
	}
	entries = append(entries, fileInfos...)

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Type < entries[j].Type
		}
		return entries[i].Path < entries[j].Path
	})

	hasMore := false
	if len(entries) > limit {
		hasMore = true
		entries = entries[:limit]
	}

	return entries, hasMore, nil
}

// Search returns chunks containing query by naive substring match.
func (storage *memoryStorageMock) Search(_ context.Context, project, query, pathPrefix string, limit int) ([]files.FileChunk, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	if limit <= 0 {
		limit = 5
	}

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

// Delete deletes one file or a directory subtree from in-memory storage.
func (storage *memoryStorageMock) Delete(_ context.Context, project, filePath string, recursive bool) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()

	key := storage.key(project, filePath)
	if recursive {
		prefix := key
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		for item := range storage.files {
			if item == key || strings.HasPrefix(item, prefix) {
				delete(storage.files, item)
			}
		}
		return nil
	}

	delete(storage.files, key)
	return nil
}
