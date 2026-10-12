package memory

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/agents/files"
	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

const (
	maxSummaryDepth         = 32
	maxSummaryDirectories   = 200
	maxSummaryAbstractBytes = 64 * 1024
	maxSummaryTotalBytes    = 1024 * 1024
	maxSummaryWireBytes     = 4 * 1024 * 1024
	summaryTimeout          = 10 * time.Second
)

// ListDirWithAbstract returns bounded directory summaries inside sessionID.
// Empty listPath selects the session root; explicit paths must be canonical
// descendants. Positive depth/limit are capped at 32/200. Backend over-return,
// excessive derived directories, abstracts over 64 KiB, aggregate abstracts over
// 1 MiB, or the ten-second deadline return an error, never partial results.
// Missing abstracts may be created, but never outside the validated subtree.
// Project and session authorization remain the embedding application's duty.
func (engine *StandardEngine) ListDirWithAbstract(
	ctx context.Context, project, sessionID, listPath string, depth, limit int,
) ([]DirectorySummary, error) {
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("project is required")
	}
	if err := validateSessionID(sessionID); err != nil {
		return nil, errors.Wrap(err, "validate summary session")
	}
	root := strings.TrimSpace(listPath)
	if root == "" {
		root = sessionBasePath(sessionID)
	}
	if err := validateSummaryPath(sessionBasePath(sessionID), root); err != nil {
		return nil, errors.Wrap(err, "validate summary root")
	}
	if depth <= 0 {
		depth = 8
	}
	depth = min(depth, maxSummaryDepth)
	if limit <= 0 {
		limit = maxSummaryDirectories
	}
	limit = min(limit, maxSummaryDirectories)
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	// Include initialization and retries in the actual HTTP-attempt budget. Local
	// and custom backends remain subject to the bounded logical call loop below.
	ctx, err := files.WithMCPBudget(ctx, 2*(1+3*limit), maxSummaryWireBytes)
	if err != nil {
		return nil, errors.Wrap(err, "set summary MCP budget")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "summary context")
	}
	entries, _, err := engine.storage.List(ctx, project, root, depth, limit)
	if err != nil {
		return nil, errors.Wrap(err, "list summary entries")
	}
	if len(entries) > limit {
		return nil, errors.New("summary listing exceeds entry budget")
	}
	dirs, err := boundedSummaryDirs(root, entries, depth, limit)
	if err != nil {
		return nil, errors.Wrap(err, "validate summary listing")
	}
	result := make([]DirectorySummary, 0, len(dirs))
	remaining := int64(maxSummaryTotalBytes)
	for _, dir := range dirs {
		summary, err := engine.readDirectorySummary(ctx, project, dir, &remaining)
		if err != nil {
			return nil, errors.Wrap(err, "load directory summary")
		}
		result = append(result, summary)
	}
	return result, nil
}

// validateSummaryPath rejects noncanonical or out-of-subtree paths before I/O.
// The length ceiling leaves room for the appended /.abstract and /.overview.
func validateSummaryPath(root, p string) error {
	if len(p) > 502 || !utf8.ValidString(p) || !strings.HasPrefix(p, "/") ||
		path.Clean(p) != p || strings.Contains(p, "\\") {
		return errors.New("summary path must be canonical and bounded")
	}
	for _, r := range p {
		if r <= 0x20 || r == 0x7f {
			return errors.New("summary path contains whitespace or controls")
		}
	}
	if p != root && !strings.HasPrefix(p, root+"/") {
		return errors.New("summary path is outside the requested subtree")
	}
	return nil
}

// boundedSummaryDirs validates all backend paths before deriving a bounded,
// sorted directory set. No metadata reads or writes occur during validation.
func boundedSummaryDirs(root string, entries []storageengine.FileInfo, depth, limit int) ([]string, error) {
	dirs := map[string]struct{}{root: {}}
	for _, entry := range entries {
		if err := validateSummaryPath(root, entry.Path); err != nil {
			return nil, errors.WithStack(err)
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(entry.Path, root), "/")
		if relative != "" && strings.Count(relative, "/")+1 > depth {
			return nil, errors.New("summary listing exceeds depth budget")
		}
		current := entry.Path
		if entry.Type != storageengine.FileTypeDirectory {
			current = path.Dir(current)
		}
		for current == root || strings.HasPrefix(current, root+"/") {
			dirs[current] = struct{}{}
			if len(dirs) > limit {
				return nil, errors.New("summary directory fanout exceeds budget")
			}
			if current == root {
				break
			}
			current = path.Dir(current)
		}
	}
	result := make([]string, 0, len(dirs))
	for dir := range dirs {
		result = append(result, dir)
	}
	sort.Strings(result)
	return result, nil
}

// readDirectorySummary performs at most three bounded storage calls for dir,
// charging untrimmed abstract bytes to the shared remaining response budget.
func (engine *StandardEngine) readDirectorySummary(
	ctx context.Context, project, dir string, remaining *int64,
) (DirectorySummary, error) {
	if err := ctx.Err(); err != nil {
		return DirectorySummary{}, errors.Wrap(err, "summary context")
	}
	abstractPath := path.Join(dir, ".abstract")
	info, err := engine.storage.Stat(ctx, project, abstractPath)
	if err != nil {
		return DirectorySummary{}, errors.Wrap(err, "stat summary abstract")
	}
	if err := ctx.Err(); err != nil {
		return DirectorySummary{}, errors.Wrap(err, "summary context")
	}
	var body string
	updatedAt := ""
	if info.Exists && info.Type == storageengine.FileTypeFile {
		allowance := min(int64(maxSummaryAbstractBytes), *remaining)
		if info.SizeBytes < 0 || info.SizeBytes > allowance {
			return DirectorySummary{}, errors.New("summary abstract exceeds byte budget")
		}
		body, err = engine.storage.Read(ctx, project, abstractPath, 0, allowance+1)
		if err != nil {
			return DirectorySummary{}, errors.Wrap(err, "read summary abstract")
		}
		updatedAt = info.UpdatedAt
	} else {
		body = buildDefaultAbstract(dir)
	}
	if int64(len(body)) > *remaining || len(body) > maxSummaryAbstractBytes {
		return DirectorySummary{}, errors.New("summary abstracts exceed byte budget")
	}
	*remaining -= int64(len(body))
	if err := ctx.Err(); err != nil {
		return DirectorySummary{}, errors.Wrap(err, "summary context")
	}
	if !info.Exists || info.Type != storageengine.FileTypeFile {
		if err := engine.storage.Write(
			ctx, project, abstractPath, body, storageengine.WriteModeTruncate, 0,
		); err != nil {
			return DirectorySummary{}, errors.Wrap(err, "create summary abstract")
		}
	}
	if err := ctx.Err(); err != nil {
		return DirectorySummary{}, errors.Wrap(err, "summary context")
	}
	overview, err := engine.storage.Stat(ctx, project, path.Join(dir, ".overview"))
	if err != nil {
		return DirectorySummary{}, errors.Wrap(err, "stat summary overview")
	}
	if err := ctx.Err(); err != nil {
		return DirectorySummary{}, errors.Wrap(err, "summary context")
	}
	return DirectorySummary{
		Path: dir, Abstract: strings.TrimSpace(body), UpdatedAt: updatedAt,
		HasOverview: overview.Exists && overview.Type == storageengine.FileTypeFile,
	}, nil
}
