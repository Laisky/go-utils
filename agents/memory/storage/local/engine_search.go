package local

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"

	memorystorage "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// Search finds case-insensitive substrings under pathPrefix. In addition to the
// result count, it bounds discovered entries, files, scanned/retained bytes,
// depth, and duration. Budget exhaustion or cancellation returns an error and
// no partial results. Symlinks, special files and files over 4 MiB are skipped.
func (engine *Engine) Search(ctx context.Context, project, query, pathPrefix string, limit int) ([]memorystorage.FileChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "search context")
	}
	if strings.TrimSpace(query) == "" || len(query) > 4096 {
		return nil, errors.New("search query must contain 1 to 4096 bytes")
	}
	prefix, _, err := normalizePath(pathPrefix, true)
	if err != nil {
		return nil, errors.Wrap(err, "normalize search prefix")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	ctx, cancel := context.WithTimeout(ctx, engine.limits.SearchTimeout)
	defer cancel()
	root, err := engine.openProjectRoot(project, false)
	if err != nil {
		if errors.Is(err, errProjectRootNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "open search project")
	}
	if root == nil {
		return nil, nil
	}
	state := &searchState{ctx: ctx, root: root, limits: engine.limits, query: query, queryLower: strings.ToLower(query), resultLimit: limit, entries: 1}
	walkErr := state.walk(prefix, 0)
	closeErr := root.Close()
	if closeErr != nil {
		return nil, errors.Join(walkErr, errors.Wrap(closeErr, "close search project"))
	}
	if walkErr != nil && !errors.Is(walkErr, errWalkStop) {
		return nil, errors.Wrap(walkErr, "search project")
	}
	sort.SliceStable(state.chunks, func(i, j int) bool { return state.chunks[i].FilePath < state.chunks[j].FilePath })
	return state.chunks, nil
}

// searchState owns the counters and detached results for one bounded operation.
type searchState struct {
	ctx                         context.Context
	root                        *os.Root
	limits                      Limits
	query, queryLower           string
	resultLimit, files, entries int
	scanned, retained           int64
	chunks                      []memorystorage.FileChunk
}

// walk traverses a single rooted path with bounded directory enumeration and
// recursion. Discovery itself counts toward the entry budget, even for skips.
func (state *searchState) walk(current string, depth int) error {
	if err := state.ctx.Err(); err != nil {
		return errors.Wrap(err, "search context")
	}
	if depth > maxListDepth {
		return errors.Wrap(ErrBudgetExceeded, "search depth limit")
	}
	info, err := state.root.Lstat(current)
	if err != nil {
		if depth == 0 && os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "stat search entry")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !info.IsDir() {
		return state.scanFile(current, info)
	}
	entries, err := state.readDirectory(current)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, entry := range entries {
		if err := state.walk(path.Join(current, entry.Name()), depth+1); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// readDirectory enumerates bounded batches instead of fs.WalkDir's unbounded
// whole-directory allocation. Sorting preserves deterministic matching order.
func (state *searchState) readDirectory(current string) (entries []fs.DirEntry, err error) {
	dir, err := state.root.Open(current)
	if err != nil {
		return nil, errors.Wrap(err, "open search directory")
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil {
			entries = nil
			err = errors.Join(err, errors.Wrap(closeErr, "close search directory"))
		}
	}()
	for {
		if err := state.ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "search directory context")
		}
		batch, readErr := dir.ReadDir(min(64, state.limits.MaxSearchEntries-state.entries+1))
		if len(batch) > state.limits.MaxSearchEntries-state.entries {
			return nil, errors.Wrap(ErrBudgetExceeded, "search entry limit")
		}
		state.entries += len(batch)
		entries = append(entries, batch...)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, errors.Wrap(readErr, "read search directory")
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// scanFile scans one regular file with a bounded range reader. Actual read
// bytes and retained content consume independent aggregate operation budgets.
func (state *searchState) scanFile(current string, info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return nil
	}
	if state.files >= state.limits.MaxSearchFiles {
		return errors.Wrap(ErrBudgetExceeded, "search file limit")
	}
	state.files++
	if info.Size() > maxSearchFileBytes {
		return nil
	}
	available := state.limits.MaxSearchBytes - state.scanned
	body, err := readRootRange(state.ctx, state.root, current, 0, -1, min(int64(maxSearchFileBytes), available))
	if err != nil {
		return errors.Wrap(err, "read search file")
	}
	state.scanned += int64(len(body))
	idx := strings.Index(strings.ToLower(body), state.queryLower)
	if idx < 0 {
		return nil
	}
	if int64(len(body)) > state.limits.MaxSearchResultBytes-state.retained {
		return errors.Wrap(ErrBudgetExceeded, "search result byte limit")
	}
	state.retained += int64(len(body))
	state.chunks = append(state.chunks, memorystorage.FileChunk{FilePath: toStoragePath(current), StartBytes: int64(idx), EndBytes: int64(idx + len(state.query)), Content: body, Score: 0.9})
	if len(state.chunks) >= state.resultLimit {
		return errWalkStop
	}
	return nil
}
