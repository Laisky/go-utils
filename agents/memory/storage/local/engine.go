package local

import (
	"context"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

var (
	errWalkStop            = errors.New("walk stopped because result limit was reached")
	errProjectRootNotFound = errors.New("project root not found")
)

const (
	// defaultListDepth is the fallback traversal depth when callers pass non-positive depth.
	defaultListDepth = 8
	// defaultListLimit is the fallback maximum entry count when callers pass non-positive limit.
	defaultListLimit = 1000
	// defaultSearchLimit is the fallback maximum chunk count when callers pass non-positive limit.
	defaultSearchLimit = 5
	// maxListDepth is the hard upper bound for list traversal depth.
	maxListDepth = 32
	// maxListLimit is the hard upper bound for list result count.
	maxListLimit = 5000
	// maxSearchLimit is the hard upper bound for search result count.
	maxSearchLimit = 50
	// maxSearchFileBytes is the maximum file size allowed for content-based search scanning.
	maxSearchFileBytes = 4 * 1024 * 1024
)

// Config controls Local storage engine initialization.
type Config struct {
	RootDir  string
	FilePerm os.FileMode
	DirPerm  os.FileMode
}

// Engine is a local filesystem implementation of the memory storage interface.
type Engine struct {
	root     *os.Root
	filePerm os.FileMode
	dirPerm  os.FileMode
}

// NewEngine creates a local storage engine rooted at conf.RootDir.
//
// Parameters:
//   - conf: Local engine configuration including root path and file permissions.
//
// Returns:
//   - *Engine: The initialized local storage engine.
//   - error: Non-nil when the root path is invalid or cannot be opened.
func NewEngine(conf Config) (*Engine, error) {
	if strings.TrimSpace(conf.RootDir) == "" {
		return nil, errors.Errorf("root_dir is required")
	}

	dirPerm := conf.DirPerm
	if dirPerm == 0 {
		// Security: default to owner-only (0o700) so the storage tree is not
		// group/world-readable or traversable. Memory storage can hold sensitive
		// conversation history and extracted facts; on shared hosts a permissive
		// 0o755 default would expose it to other local users. Callers may still
		// override via Config for environments that require wider access.
		dirPerm = 0o700
	}

	// Create the root with the (tightened) directory permission rather than a
	// hard-coded 0o755, so the root itself is not world-traversable by default.
	if err := os.MkdirAll(conf.RootDir, dirPerm); err != nil {
		return nil, errors.Wrap(err, "mkdir local storage root")
	}

	root, err := os.OpenRoot(conf.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "open local storage root")
	}

	filePerm := conf.FilePerm
	if filePerm == 0 {
		// Security: default to owner-only read/write (0o600) so stored files are
		// not readable by group/other on shared systems.
		filePerm = 0o600
	}

	return &Engine{
		root:     root,
		filePerm: filePerm,
		dirPerm:  dirPerm,
	}, nil
}

// Close releases the underlying os.Root handle.
//
// Parameters:
//   - none.
//
// Returns:
//   - error: Non-nil when closing the root handle fails.
func (engine *Engine) Close() error {
	if engine == nil || engine.root == nil {
		return nil
	}

	if err := engine.root.Close(); err != nil {
		return errors.Wrap(err, "close local root")
	}

	return nil
}

// Read reads content from a file path with optional byte slicing.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path such as /memory/session/context.jsonl.
//   - offset: Start byte offset in the target file.
//   - length: Number of bytes to read; -1 means read to EOF.
//
// Returns:
//   - string: The requested file content slice, or empty string when file does not exist.
//   - error: Non-nil when validation fails or filesystem access fails.
func (engine *Engine) Read(ctx context.Context, project, storagePath string, offset, length int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return "", errors.Wrap(err, "normalize path")
	}

	if offset < 0 {
		return "", errors.Errorf("offset must be >= 0")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return "", nil
		}
		return "", errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return "", nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	body, err := projectRoot.ReadFile(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(err, "read file")
	}

	if offset > int64(len(body)) {
		return "", errors.Errorf("offset out of range")
	}

	body = body[offset:]
	if length >= 0 && length < int64(len(body)) {
		body = body[:length]
	}

	return string(body), nil
}

// Write writes content into a file using append, overwrite, or truncate mode.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path such as /memory/session/context.jsonl.
//   - content: UTF-8 text content to write.
//   - mode: Write mode controlling append/overwrite/truncate behavior.
//   - offset: Start offset for overwrite mode.
//
// Returns:
//   - error: Non-nil when validation fails or write operations fail.
func (engine *Engine) Write(
	ctx context.Context,
	project, storagePath, content string,
	mode memorystorage.WriteMode,
	offset int64,
) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return errors.Wrap(err, "normalize path")
	}

	if offset < 0 {
		return errors.Errorf("offset must be >= 0")
	}

	projectRoot, err := engine.openProjectRoot(project, true)
	if err != nil {
		return errors.Wrap(err, "open project root")
	}
	if errors.Is(err, errProjectRootNotFound) {
		return errors.Errorf("project root is nil")
	}
	if projectRoot == nil {
		return errors.Errorf("project root is nil")
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	parentDir := path.Dir(relPath)
	if parentDir != "." {
		if err = projectRoot.MkdirAll(parentDir, engine.dirPerm); err != nil {
			return errors.Wrap(err, "mkdir parent directories")
		}
	}

	switch mode {
	case memorystorage.WriteModeAppend:
		if err = writeAppend(projectRoot, relPath, content, engine.filePerm); err != nil {
			return errors.Wrap(err, "append content")
		}
	case memorystorage.WriteModeOverwrite:
		if err = writeOverwrite(projectRoot, relPath, content, offset, engine.filePerm); err != nil {
			return errors.Wrap(err, "overwrite content")
		}
	case memorystorage.WriteModeTruncate:
		if err = projectRoot.WriteFile(relPath, []byte(content), engine.filePerm); err != nil {
			return errors.Wrap(err, "truncate and write content")
		}
	default:
		return errors.Errorf("unsupported write mode `%s`", mode)
	}

	return nil
}

// Stat returns metadata for a target path inside one project namespace.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path or root marker (/ or empty when allow-root callers use it).
//
// Returns:
//   - memorystorage.FileInfo: Metadata describing existence, type, size, and update time.
//   - error: Non-nil when validation fails or stat operations fail.
func (engine *Engine) Stat(ctx context.Context, project, storagePath string) (memorystorage.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return memorystorage.FileInfo{}, errors.Wrap(err, "context done")
	}

	relPath, normalizedPath, err := normalizePath(storagePath, true)
	if err != nil {
		return memorystorage.FileInfo{}, errors.Wrap(err, "normalize path")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return memorystorage.FileInfo{
				Path:   normalizedPath,
				Exists: false,
				Type:   memorystorage.FileTypeUnknown,
			}, nil
		}
		return memorystorage.FileInfo{}, errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return memorystorage.FileInfo{Path: normalizedPath, Exists: false, Type: memorystorage.FileTypeUnknown}, nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	fileInfo, err := projectRoot.Stat(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return memorystorage.FileInfo{Path: normalizedPath, Exists: false, Type: memorystorage.FileTypeUnknown}, nil
		}
		return memorystorage.FileInfo{}, errors.Wrap(err, "stat path")
	}

	return memorystorage.FileInfo{
		Path:      normalizedPath,
		Exists:    true,
		Type:      fileTypeFromMode(fileInfo.Mode()),
		SizeBytes: fileInfo.Size(),
		UpdatedAt: fileInfo.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// List lists file and directory entries under a path.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute root path for traversal.
//   - depth: Maximum descendant depth to include.
//   - limit: Maximum number of entries to return.
//
// Returns:
//   - []memorystorage.FileInfo: Matched entries with normalized absolute storage paths.
//   - bool: True when there are more entries beyond the returned limit.
//   - error: Non-nil when validation fails or traversal fails.
func (engine *Engine) List(
	ctx context.Context,
	project, storagePath string,
	depth, limit int,
) ([]memorystorage.FileInfo, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, errors.Wrap(err, "context done")
	}

	startRelPath, _, err := normalizePath(storagePath, true)
	if err != nil {
		return nil, false, errors.Wrap(err, "normalize path")
	}

	if depth <= 0 {
		depth = defaultListDepth
	}
	if depth > maxListDepth {
		depth = maxListDepth
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return nil, false, nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	if startRelPath != "." {
		if _, err = projectRoot.Stat(startRelPath); err != nil {
			if os.IsNotExist(err) {
				return nil, false, nil
			}
			return nil, false, errors.Wrap(err, "stat list root")
		}
	}

	entries := make([]memorystorage.FileInfo, 0, minInt(limit, 256))
	hasMore := false

	walkErr := fs.WalkDir(
		projectRoot.FS(),
		startRelPath,
		func(currentPath string, entry fs.DirEntry, walkErr error) error {
			return engine.handleListWalkEntry(
				ctx,
				startRelPath,
				currentPath,
				entry,
				walkErr,
				depth,
				limit,
				&entries,
				&hasMore,
			)
		},
	)
	if walkErr != nil && !errors.Is(walkErr, errWalkStop) {
		return nil, false, errors.Wrap(walkErr, "walk list root")
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Type < entries[j].Type
		}
		return entries[i].Path < entries[j].Path
	})

	return entries, hasMore, nil
}

// Search searches file contents under a path prefix and returns matching chunks.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - query: Case-insensitive substring query.
//   - pathPrefix: Absolute prefix path used to limit traversal scope.
//   - limit: Maximum number of chunks to return.
//
// Returns:
//   - []memorystorage.FileChunk: Matching chunks with path and byte range metadata.
//   - error: Non-nil when validation fails or traversal/read operations fail.
func (engine *Engine) Search(
	ctx context.Context,
	project, query, pathPrefix string,
	limit int,
) ([]memorystorage.FileChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}

	if strings.TrimSpace(query) == "" {
		return nil, errors.Errorf("query is required")
	}

	prefixRelPath, _, err := normalizePath(pathPrefix, true)
	if err != nil {
		return nil, errors.Wrap(err, "normalize path prefix")
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return nil, nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	if prefixRelPath != "." {
		if _, err = projectRoot.Stat(prefixRelPath); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, errors.Wrap(err, "stat search root")
		}
	}

	queryLower := strings.ToLower(query)
	chunks := make([]memorystorage.FileChunk, 0, minInt(limit, 32))

	walkErr := fs.WalkDir(
		projectRoot.FS(),
		prefixRelPath,
		func(currentPath string, entry fs.DirEntry, walkErr error) error {
			return engine.handleSearchWalkEntry(
				ctx,
				currentPath,
				entry,
				walkErr,
				projectRoot,
				query,
				queryLower,
				limit,
				&chunks,
			)
		},
	)
	if walkErr != nil && !errors.Is(walkErr, errWalkStop) {
		return nil, errors.Wrap(walkErr, "walk search root")
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		return chunks[i].FilePath < chunks[j].FilePath
	})

	return chunks, nil
}

// handleListWalkEntry processes one WalkDir entry for List.
func (engine *Engine) handleListWalkEntry(
	ctx context.Context,
	startRelPath, currentPath string,
	entry fs.DirEntry,
	walkErr error,
	depth, limit int,
	entries *[]memorystorage.FileInfo,
	hasMore *bool,
) error {
	if walkErr != nil {
		return wrapTraversalError("listing", currentPath, walkErr)
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done while listing")
	}

	relativeDepth := calculateRelativeDepth(startRelPath, currentPath)
	if relativeDepth == 0 {
		return nil
	}
	if relativeDepth > depth {
		if entry.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if isSymlinkEntry(entry) {
		return nil
	}

	fileInfo, err := entry.Info()
	if err != nil {
		return wrapEntryInfoError(currentPath, err)
	}

	*entries = append(*entries, memorystorage.FileInfo{
		Path:      toStoragePath(currentPath),
		Exists:    true,
		Type:      fileTypeFromMode(fileInfo.Mode()),
		SizeBytes: fileInfo.Size(),
		UpdatedAt: fileInfo.ModTime().UTC().Format(time.RFC3339),
	})
	if len(*entries) >= limit {
		*hasMore = true
		return errWalkStop
	}

	return nil
}

// handleSearchWalkEntry processes one WalkDir entry for Search.
func (engine *Engine) handleSearchWalkEntry(
	ctx context.Context,
	currentPath string,
	entry fs.DirEntry,
	walkErr error,
	projectRoot *os.Root,
	query, queryLower string,
	limit int,
	chunks *[]memorystorage.FileChunk,
) error {
	if walkErr != nil {
		return wrapTraversalError("searching", currentPath, walkErr)
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done while searching")
	}
	if entry.IsDir() || isSymlinkEntry(entry) {
		return nil
	}

	fileInfo, err := entry.Info()
	if err != nil {
		return wrapEntryInfoError(currentPath, err)
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Size() > maxSearchFileBytes {
		return nil
	}

	body, err := projectRoot.ReadFile(currentPath)
	if err != nil {
		return wrapSearchReadError(currentPath, err)
	}

	content := string(body)
	idx := strings.Index(strings.ToLower(content), queryLower)
	if idx < 0 {
		return nil
	}

	*chunks = append(*chunks, memorystorage.FileChunk{
		FilePath:   toStoragePath(currentPath),
		StartBytes: int64(idx),
		EndBytes:   int64(idx + len(query)),
		Content:    content,
		Score:      0.9,
	})
	if len(*chunks) >= limit {
		return errWalkStop
	}

	return nil
}

// Delete deletes one path or recursively removes a directory subtree.
//
// Parameters:
//   - ctx: Request-scoped context used for cancellation checks.
//   - project: Project namespace under the configured local root.
//   - storagePath: Absolute storage path to delete.
//   - recursive: Whether directory deletion should remove descendants.
//
// Returns:
//   - error: Non-nil when validation fails or deletion fails.
func (engine *Engine) Delete(ctx context.Context, project, storagePath string, recursive bool) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}

	relPath, _, err := normalizePath(storagePath, false)
	if err != nil {
		return errors.Wrap(err, "normalize path")
	}

	projectRoot, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return nil
		}
		return errors.Wrap(err, "open project root")
	}
	if projectRoot == nil {
		return nil
	}
	defer func() {
		_ = projectRoot.Close()
	}()

	if recursive {
		if err = projectRoot.RemoveAll(relPath); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "remove all path")
		}
		return nil
	}

	if err = projectRoot.Remove(relPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove path")
	}

	return nil
}

// openProjectRoot opens one project namespace root and optionally creates it first.
//
// Parameters:
//   - project: Project namespace name.
//   - create: Whether missing project directory should be created.
//
// Returns:
//   - *os.Root: Opened project root, or nil when project does not exist and create=false.
//   - error: Non-nil when project validation or root operations fail.
func (engine *Engine) openProjectRoot(project string, create bool) (*os.Root, error) {
	if err := validateProject(project); err != nil {
		return nil, errors.Wrap(err, "validate project")
	}

	if create {
		if err := engine.root.MkdirAll(project, engine.dirPerm); err != nil {
			return nil, errors.Wrap(err, "mkdir project root")
		}
	}

	projectRoot, err := engine.root.OpenRoot(project)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errProjectRootNotFound
		}
		return nil, errors.Wrap(err, "open project root")
	}

	return projectRoot, nil
}
