package local

import (
	"context"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

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
