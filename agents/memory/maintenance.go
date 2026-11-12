package memory

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// RunMaintenance runs compaction, retention sweeping, and summary refresh for one session.
// Independent retention and summaries continue after ordinary stage failures.
// Consolidation and the success timestamp require all preceding stages to succeed.
func (engine *StandardEngine) RunMaintenance(ctx context.Context, project, sessionID string) error {
	if strings.TrimSpace(project) == "" {
		return errors.Errorf("project is required")
	}
	// Security: reject traversal-capable session IDs before any per-session path
	// is built, preventing cross-session namespace escape under /memory/<session>/.
	if err := validateSessionID(sessionID); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "maintenance canceled")
	}

	if err := engine.ensureSessionScaffold(ctx, project, sessionID); err != nil {
		return errors.Wrap(err, "ensure session scaffold")
	}

	contextEvents, err := engine.loadContextEventsWithFallback(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load context for maintenance")
	}
	if len(contextEvents) > engine.conf.RecentContextItems*2 {
		if err = engine.compactRuntimeContext(ctx, project, sessionID, contextEvents, 0); err != nil {
			return errors.Wrap(err, "compact runtime context")
		}
	}

	// Keep the original successful ordering. Retained raw shards and partially
	// swept facts are not valid inputs for the usual consolidation cycle.
	var stageErrors []error
	// runStage records a named failure and stops the pipeline on cancellation.
	runStage := func(name string, run func() error) bool {
		if ctxErr := ctx.Err(); ctxErr != nil {
			stageErrors = append(stageErrors, errors.Wrap(ctxErr, "maintenance canceled"))
			return false
		}
		if stageErr := run(); stageErr != nil {
			stageErrors = append(stageErrors, errors.Wrap(stageErr, name))
			if errors.Is(stageErr, context.Canceled) || errors.Is(stageErr, context.DeadlineExceeded) {
				return false
			}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			stageErrors = append(stageErrors, errors.Wrap(ctxErr, "maintenance canceled"))
			return false
		}
		return true
	}
	if !runStage("archive raw shards", func() error { return engine.archiveRawShards(ctx, project, sessionID) }) {
		return errors.Join(stageErrors...)
	}
	if !runStage("sweep expired L1 facts", func() error {
		return engine.sweepExpiredTierFacts(ctx, project, sessionID, memoryTierL1)
	}) {
		return errors.Join(stageErrors...)
	}
	if !runStage("sweep expired L2 facts", func() error {
		return engine.sweepExpiredTierFacts(ctx, project, sessionID, memoryTierL2)
	}) {
		return errors.Join(stageErrors...)
	}
	if len(stageErrors) == 0 {
		if !runStage("run consolidation", func() error { return engine.RunConsolidation(ctx, project, sessionID) }) {
			return errors.Join(stageErrors...)
		}
	}
	// Summaries only describe current directory state; they do not require a
	// successful archive or consolidation and are safe after partial cleanup.
	if !runStage("refresh summaries", func() error { return engine.refreshSummaries(ctx, project, sessionID) }) {
		return errors.Join(stageErrors...)
	}
	if len(stageErrors) != 0 {
		return errors.Join(stageErrors...)
	}

	meta, err := engine.loadMeta(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load meta")
	}
	meta.LastMaintenanceAt = engine.conf.TimeNow().UTC().Format(time.RFC3339)
	meta.UpdatedAt = meta.LastMaintenanceAt
	if err = engine.writeMeta(ctx, project, sessionID, meta); err != nil {
		return errors.Wrap(err, "write meta")
	}

	return nil
}

// archiveRawShards archives old raw log shards and records compact summary events.
func (engine *StandardEngine) archiveRawShards(ctx context.Context, project, sessionID string) error {
	filesInfo, err := engine.listFiles(ctx, project, eventsRawRootPath(sessionID), ".jsonl")
	if err != nil {
		return errors.Wrap(err, "list raw shards")
	}

	now := engine.conf.TimeNow().UTC()
	for _, info := range filesInfo {
		shardDate, ok := parseRawShardDate(info.Path)
		if !ok {
			continue
		}
		if now.Sub(shardDate) < engine.conf.CompactionMinAge {
			continue
		}

		body, readErr := engine.storage.Read(ctx, project, info.Path, 0, -1)
		if readErr != nil {
			return errors.Wrapf(readErr, "read raw shard %s", info.Path)
		}
		if strings.TrimSpace(body) == "" {
			if delErr := engine.storage.Delete(ctx, project, info.Path, false); delErr != nil {
				return errors.Wrapf(delErr, "delete empty raw shard %s", info.Path)
			}
			continue
		}

		compressed, compressErr := compressZstd(body)
		if compressErr != nil {
			return errors.Wrapf(compressErr, "compress raw shard %s", info.Path)
		}

		archivePath := archiveShardPath(sessionID, shardDate, path.Base(info.Path))
		if engine.conf.ArchiveFormat == ArchiveFormatZstdBase64V1 {
			archivePath += ".b64.json"
		}
		if writeErr := engine.writeVerifiedArchive(ctx, project, archivePath, compressed); writeErr != nil {
			return errors.Wrapf(writeErr, "persist archive shard %s", archivePath)
		}
		if delErr := engine.storage.Delete(ctx, project, info.Path, false); delErr != nil {
			return errors.Wrapf(delErr, "delete raw shard %s", info.Path)
		}

		summaryEvent := LogEvent{
			ID:      "archive-" + now.Format("20060102T150405.000000000"),
			TS:      now.Format(time.RFC3339),
			Type:    "compact_summary",
			Summary: "archived raw shard " + info.Path,
		}
		if appendErr := engine.appendJSONL(
			ctx,
			project,
			compactShardPath(sessionID, now),
			[]LogEvent{summaryEvent},
		); appendErr != nil {
			return errors.Wrap(appendErr, "append archive compact summary")
		}
	}

	return nil
}

// parseRawShardDate parses shard date from raw event path and returns parsed day and parse result.
func parseRawShardDate(filePath string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(filePath), "/")
	if len(parts) < 4 {
		return time.Time{}, false
	}
	for idx := 0; idx+2 < len(parts); idx++ {
		year := parts[idx]
		month := parts[idx+1]
		day := parts[idx+2]
		if len(year) != 4 || len(month) != 2 || len(day) != 2 {
			continue
		}
		dateValue, err := time.Parse("2006-01-02", year+"-"+month+"-"+day)
		if err != nil {
			continue
		}
		return dateValue.UTC(), true
	}

	return time.Time{}, false
}

// sweepExpiredTierFacts removes expired records from a specific tier and rewrites affected shards.
func (engine *StandardEngine) sweepExpiredTierFacts(ctx context.Context, project, sessionID, tier string) error {
	fileInfos, err := engine.listFiles(ctx, project, tierRootPath(sessionID, tier), ".jsonl")
	if err != nil {
		return errors.Wrap(err, "list tier files")
	}

	now := engine.conf.TimeNow().UTC()
	for _, info := range fileInfos {
		facts, readErr := engine.loadFactsFromFile(ctx, project, info.Path)
		if readErr != nil {
			return errors.Wrapf(readErr, "load tier file %s", info.Path)
		}

		active := make([]MemoryFact, 0, len(facts))
		for _, fact := range facts {
			if isFactExpired(now, fact) {
				continue
			}
			active = append(active, fact)
		}

		if len(active) == len(facts) {
			continue
		}
		if len(active) == 0 {
			if delErr := engine.storage.Delete(ctx, project, info.Path, false); delErr != nil {
				return errors.Wrapf(delErr, "delete expired tier file %s", info.Path)
			}
			continue
		}

		body, marshalErr := marshalFacts(active)
		if marshalErr != nil {
			return errors.Wrapf(marshalErr, "marshal active facts for %s", info.Path)
		}
		if writeErr := engine.storage.Write(
			ctx,
			project,
			info.Path,
			body,
			storageengine.WriteModeTruncate,
			0,
		); writeErr != nil {
			return errors.Wrapf(writeErr, "rewrite tier file %s", info.Path)
		}
	}

	return nil
}

// refreshSummaries refreshes abstract and overview files for known directories.
func (engine *StandardEngine) refreshSummaries(ctx context.Context, project, sessionID string) error {
	for _, dir := range knownSummaryDirs(sessionID) {
		if err := engine.refreshDirectorySummary(ctx, project, dir); err != nil {
			return errors.Wrapf(err, "refresh summary for %s", dir)
		}
	}

	return nil
}

// refreshDirectorySummary rewrites abstract and overview files based on current folder state.
func (engine *StandardEngine) refreshDirectorySummary(ctx context.Context, project, dir string) error {
	entries, _, err := engine.storage.List(ctx, project, dir, 3, 200)
	if err != nil {
		return errors.Wrap(err, "list directory entries")
	}

	abstractPath := path.Join(dir, ".abstract")
	overviewPath := path.Join(dir, ".overview")

	abstract := buildDefaultAbstract(dir)
	if wordCount(abstract) > 200 {
		parts := strings.Fields(abstract)
		abstract = strings.Join(parts[:200], " ")
	}

	overview := engine.buildOverviewFromEntries(dir, entries)
	if wordCount(overview) > 2000 {
		parts := strings.Fields(overview)
		overview = strings.Join(parts[:2000], " ")
	}

	if err = engine.storage.Write(
		ctx,
		project,
		abstractPath,
		abstract,
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
		return errors.Wrap(err, "write abstract")
	}
	if err = engine.storage.Write(
		ctx,
		project,
		overviewPath,
		overview,
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
		return errors.Wrap(err, "write overview")
	}

	return nil
}

// buildOverviewFromEntries builds directory overview text from current entries and returns the rendered summary.
func (engine *StandardEngine) buildOverviewFromEntries(dir string, entries []storageengine.FileInfo) string {
	fileCount := 0
	dirCount := 0
	unknownCount := 0
	totalBytes := int64(0)
	paths := make([]string, 0, len(entries))

	for _, entry := range entries {
		switch entry.Type {
		case storageengine.FileTypeFile:
			fileCount++
			totalBytes += entry.SizeBytes
		case storageengine.FileTypeDirectory:
			dirCount++
		default:
			unknownCount++
		}
		paths = append(paths, entry.Path)
	}

	sort.Strings(paths)
	if len(paths) > 20 {
		paths = paths[:20]
	}

	payload := map[string]any{
		"generated_at": engine.conf.TimeNow().UTC().Format(time.RFC3339),
		"path":         dir,
		"stats": map[string]any{
			"files":       fileCount,
			"directories": dirCount,
			"unknown":     unknownCount,
			"total_bytes": totalBytes,
		},
		"sample_paths": paths,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return buildDefaultOverview(dir)
	}

	return "Directory overview snapshot: " + string(buf)
}
