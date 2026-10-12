package local

import (
	"context"
	"io/fs"
	"os"
	"sort"
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
