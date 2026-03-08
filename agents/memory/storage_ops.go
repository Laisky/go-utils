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

// appendJSONL marshals records into JSONL and appends them to path.
func (engine *StandardEngine) appendJSONL(ctx context.Context, project, filePath string, records any) error {
	body, err := marshalJSONL(records)
	if err != nil {
		return errors.Wrap(err, "marshal jsonl")
	}
	if body == "" {
		return nil
	}

	if err = engine.storage.Write(ctx, project, filePath, body, storageengine.WriteModeAppend, 0); err != nil {
		return errors.Wrap(err, "append jsonl")
	}

	return nil
}

// loadJSONL loads one JSONL file and returns non-empty lines while handling missing files.
func (engine *StandardEngine) loadJSONL(ctx context.Context, project, filePath string) ([]string, error) {
	info, err := engine.storage.Stat(ctx, project, filePath)
	if err != nil {
		return nil, errors.Wrap(err, "stat file")
	}
	if !info.Exists || info.Type != storageengine.FileTypeFile {
		return nil, nil
	}

	body, err := engine.storage.Read(ctx, project, filePath, 0, -1)
	if err != nil {
		return nil, errors.Wrap(err, "read file")
	}

	return parseJSONLLines(body), nil
}

// loadContextEvents decodes context events from a file path and returns parsed events.
func (engine *StandardEngine) loadContextEvents(ctx context.Context, project, filePath string) ([]LogEvent, error) {
	records, err := engine.loadJSONL(ctx, project, filePath)
	if err != nil {
		return nil, errors.Wrap(err, "load jsonl")
	}

	events := make([]LogEvent, 0, len(records))
	for _, line := range records {
		var event LogEvent
		if err = json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// loadContextEventsWithFallback loads canonical runtime context first and falls back to legacy context.
func (engine *StandardEngine) loadContextEventsWithFallback(ctx context.Context, project, sessionID string) ([]LogEvent, error) {
	events, err := engine.loadContextEvents(ctx, project, runtimeContextPath(sessionID))
	if err != nil {
		return nil, errors.Wrap(err, "load runtime context")
	}
	if len(events) > 0 {
		return events, nil
	}

	events, err = engine.loadContextEvents(ctx, project, legacyContextPath(sessionID))
	if err != nil {
		return nil, errors.Wrap(err, "load legacy context")
	}

	return events, nil
}

// loadFactsFromFile decodes memory facts from a file path and returns parsed facts.
func (engine *StandardEngine) loadFactsFromFile(ctx context.Context, project, filePath string) ([]MemoryFact, error) {
	records, err := engine.loadJSONL(ctx, project, filePath)
	if err != nil {
		return nil, errors.Wrap(err, "load facts jsonl")
	}

	facts := make([]MemoryFact, 0, len(records))
	for _, line := range records {
		var fact MemoryFact
		if err = json.Unmarshal([]byte(line), &fact); err != nil {
			continue
		}
		facts = append(facts, fact)
	}

	return facts, nil
}

// loadRecallFacts loads active facts from tiered files, ranks them for query relevance, and falls back to legacy facts when needed.
func (engine *StandardEngine) loadRecallFacts(ctx context.Context, project, sessionID, query string) ([]MemoryFact, error) {
	activeIndex, err := engine.loadActiveFactsIndex(ctx, project, sessionID)
	if err == nil && len(activeIndex.Facts) > 0 {
		facts := make([]MemoryFact, 0, len(activeIndex.Facts))
		now := engine.conf.TimeNow().UTC()
		for _, fact := range activeIndex.Facts {
			if isFactExpired(now, fact) {
				continue
			}
			facts = append(facts, fact)
		}
		facts = rankFactsForRecall(now, facts, query)
		facts = deduplicateFacts(facts)
		if len(facts) > engine.conf.RecallFactsLimit {
			facts = facts[:engine.conf.RecallFactsLimit]
		}

		return facts, nil
	}

	now := engine.conf.TimeNow().UTC()
	facts := make([]MemoryFact, 0, engine.conf.RecallFactsLimit)
	for _, tier := range []string{memoryTierL0, memoryTierL2, memoryTierL1} {
		tierFacts, err := engine.loadTierFacts(ctx, project, sessionID, tier)
		if err != nil {
			return nil, errors.Wrapf(err, "load tier facts %s", tier)
		}
		for _, fact := range tierFacts {
			if isFactExpired(now, fact) {
				continue
			}
			facts = append(facts, fact)
		}
	}

	if len(facts) == 0 {
		legacyFacts, err := engine.loadFactsFromFile(ctx, project, legacyFactsPath(sessionID))
		if err != nil {
			return nil, errors.Wrap(err, "load legacy facts")
		}
		for _, fact := range legacyFacts {
			if isFactExpired(now, fact) {
				continue
			}
			facts = append(facts, fact)
		}
	}

	facts = rankFactsForRecall(now, facts, query)
	facts = deduplicateFacts(facts)
	if len(facts) > engine.conf.RecallFactsLimit {
		facts = facts[:engine.conf.RecallFactsLimit]
	}

	return facts, nil
}

// loadTierFacts loads all facts in one tier and returns decoded records across shards.
func (engine *StandardEngine) loadTierFacts(ctx context.Context, project, sessionID, tier string) ([]MemoryFact, error) {
	root := tierRootPath(sessionID, tier)
	fileInfos, err := engine.listFiles(ctx, project, root, ".jsonl")
	if err != nil {
		return nil, errors.Wrap(err, "list tier files")
	}

	facts := make([]MemoryFact, 0, len(fileInfos)*8)
	for _, info := range fileInfos {
		tierFacts, readErr := engine.loadFactsFromFile(ctx, project, info.Path)
		if readErr != nil {
			return nil, errors.Wrapf(readErr, "load tier file %s", info.Path)
		}
		facts = append(facts, tierFacts...)
	}

	return facts, nil
}

// writeTieredFacts appends facts to tier-specific shard files grouped by target path.
func (engine *StandardEngine) writeTieredFacts(
	ctx context.Context,
	project, sessionID string,
	now time.Time,
	facts []MemoryFact,
) error {
	groups := make(map[string][]MemoryFact)
	for _, fact := range facts {
		targetPath := tierFactsShardPath(sessionID, fact.Tier, now)
		groups[targetPath] = append(groups[targetPath], fact)
	}

	paths := make([]string, 0, len(groups))
	for p := range groups {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, filePath := range paths {
		if err := engine.appendJSONL(ctx, project, filePath, groups[filePath]); err != nil {
			return errors.Wrapf(err, "write tier facts %s", filePath)
		}
	}

	return nil
}

// listFiles lists files under root, filters by suffix, and returns sorted file infos.
func (engine *StandardEngine) listFiles(ctx context.Context, project, root, suffix string) ([]storageengine.FileInfo, error) {
	entries, _, err := engine.storage.List(ctx, project, root, 16, 4096)
	if err != nil {
		return nil, errors.Wrap(err, "list entries")
	}

	filtered := make([]storageengine.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != storageengine.FileTypeFile {
			continue
		}
		if suffix != "" && !strings.HasSuffix(entry.Path, suffix) {
			continue
		}
		filtered = append(filtered, entry)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Path > filtered[j].Path
	})

	return filtered, nil
}

// loadMeta loads canonical metadata and falls back to legacy metadata file.
func (engine *StandardEngine) loadMeta(ctx context.Context, project, sessionID string) (MemoryMeta, error) {
	statePath := metaStatePath(sessionID)
	stateInfo, err := engine.storage.Stat(ctx, project, statePath)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "stat state meta")
	}

	meta, err := engine.loadMetaFile(ctx, project, statePath)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "load state meta")
	}
	if stateInfo.Exists && stateInfo.Type == storageengine.FileTypeFile {
		return meta, nil
	}

	legacy, err := engine.loadMetaFile(ctx, project, legacyMetaPath(sessionID))
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "load legacy meta")
	}
	if legacy.Version == 0 {
		legacy.Version = 1
	}

	return legacy, nil
}

// loadMetaFile loads metadata from one file path and returns default empty metadata when file is missing.
func (engine *StandardEngine) loadMetaFile(ctx context.Context, project, filePath string) (MemoryMeta, error) {
	info, err := engine.storage.Stat(ctx, project, filePath)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "stat meta file")
	}
	if !info.Exists || info.Type != storageengine.FileTypeFile {
		return MemoryMeta{Version: 1}, nil
	}

	body, err := engine.storage.Read(ctx, project, filePath, 0, -1)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "read meta file")
	}
	if strings.TrimSpace(body) == "" {
		return MemoryMeta{Version: 1}, nil
	}

	compat := memoryMetaCompat{}
	if err = json.Unmarshal([]byte(body), &compat); err != nil {
		return MemoryMeta{}, errors.Wrap(err, "unmarshal meta")
	}

	processed := compat.ProcessedTurnIDs
	if len(processed) == 0 {
		processed = compat.ProcessedTurn
	}

	meta := MemoryMeta{
		Version:           compat.Version,
		LatestTurnID:      compat.LatestTurnID,
		ProcessedTurnIDs:  processed,
		LastCompactAt:     compat.LastCompactAt,
		LastMaintenanceAt: compat.LastMaintenanceAt,
		UpdatedAt:         compat.UpdatedAt,
	}
	if meta.Version == 0 {
		meta.Version = 1
	}

	return meta, nil
}

// writeMeta writes metadata to canonical and legacy files for migration compatibility.
func (engine *StandardEngine) writeMeta(ctx context.Context, project, sessionID string, meta MemoryMeta) error {
	body, err := json.Marshal(meta)
	if err != nil {
		return errors.Wrap(err, "marshal meta")
	}

	if err = engine.storage.Write(ctx, project, metaStatePath(sessionID), string(body), storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write state meta")
	}
	if err = engine.storage.Write(ctx, project, legacyMetaPath(sessionID), string(body), storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write legacy meta")
	}

	return nil
}

// ensureSessionScaffold ensures policy and summary scaffold files exist for a session.
func (engine *StandardEngine) ensureSessionScaffold(ctx context.Context, project, sessionID string) error {
	if err := engine.ensurePolicy(ctx, project, sessionID); err != nil {
		return errors.Wrap(err, "ensure policy")
	}

	for _, dir := range knownSummaryDirs(sessionID) {
		if err := engine.ensureSummaryFiles(ctx, project, dir); err != nil {
			return errors.Wrapf(err, "ensure summary files for %s", dir)
		}
	}

	if err := engine.ensureWatermarks(ctx, project, sessionID); err != nil {
		return errors.Wrap(err, "ensure watermarks")
	}
	if err := engine.ensureMetrics(ctx, project, sessionID); err != nil {
		return errors.Wrap(err, "ensure metrics")
	}

	return nil
}

// ensurePolicy writes default policy file when it does not already exist.
func (engine *StandardEngine) ensurePolicy(ctx context.Context, project, sessionID string) error {
	policyPath := metaPolicyPath(sessionID)
	info, err := engine.storage.Stat(ctx, project, policyPath)
	if err != nil {
		return errors.Wrap(err, "stat policy")
	}
	if info.Exists && info.Type == storageengine.FileTypeFile {
		return nil
	}

	body, err := json.Marshal(defaultMemoryPolicy(engine.conf))
	if err != nil {
		return errors.Wrap(err, "marshal policy")
	}
	if err = engine.storage.Write(ctx, project, policyPath, string(body), storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write policy")
	}

	return nil
}

// ensureWatermarks writes default watermarks file when it does not already exist.
func (engine *StandardEngine) ensureWatermarks(ctx context.Context, project, sessionID string) error {
	watermarkPath := metaWatermarksPath(sessionID)
	info, err := engine.storage.Stat(ctx, project, watermarkPath)
	if err != nil {
		return errors.Wrap(err, "stat watermarks")
	}
	if info.Exists && info.Type == storageengine.FileTypeFile {
		return nil
	}

	body, err := json.Marshal(MemoryWatermarks{UpdatedAt: engine.conf.TimeNow().UTC().Format(time.RFC3339)})
	if err != nil {
		return errors.Wrap(err, "marshal watermarks")
	}

	if err = engine.storage.Write(ctx, project, watermarkPath, string(body), storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write watermarks")
	}

	return nil
}

// ensureMetrics writes a default metrics file when it does not already exist.
func (engine *StandardEngine) ensureMetrics(ctx context.Context, project, sessionID string) error {
	metricsPath := metaMetricsPath(sessionID)
	info, err := engine.storage.Stat(ctx, project, metricsPath)
	if err != nil {
		return errors.Wrap(err, "stat metrics")
	}
	if info.Exists && info.Type == storageengine.FileTypeFile {
		return nil
	}

	body, err := json.Marshal(MemoryMetrics{UpdatedAt: engine.conf.TimeNow().UTC().Format(time.RFC3339)})
	if err != nil {
		return errors.Wrap(err, "marshal metrics")
	}

	if err = engine.storage.Write(ctx, project, metricsPath, string(body), storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write metrics")
	}

	return nil
}

// ensureSummaryFiles creates placeholder abstract and overview files when they are missing.
func (engine *StandardEngine) ensureSummaryFiles(ctx context.Context, project, dir string) error {
	abstractPath := path.Join(dir, ".abstract")
	overviewPath := path.Join(dir, ".overview")

	if err := engine.ensureTextFile(ctx, project, abstractPath, buildDefaultAbstract(dir)); err != nil {
		return errors.Wrap(err, "ensure abstract")
	}
	if err := engine.ensureTextFile(ctx, project, overviewPath, buildDefaultOverview(dir)); err != nil {
		return errors.Wrap(err, "ensure overview")
	}

	return nil
}

// ensureTextFile writes content only when path does not already exist as a file.
func (engine *StandardEngine) ensureTextFile(ctx context.Context, project, filePath, content string) error {
	info, err := engine.storage.Stat(ctx, project, filePath)
	if err != nil {
		return errors.Wrap(err, "stat text file")
	}
	if info.Exists && info.Type == storageengine.FileTypeFile {
		return nil
	}

	if err = engine.storage.Write(ctx, project, filePath, content, storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write text file")
	}

	return nil
}

// buildDefaultAbstract builds a deterministic default abstract for a folder and returns 100-200 words.
func buildDefaultAbstract(dir string) string {
	text := strings.Join([]string{
		"This folder is part of the memory storage hierarchy for one agent session.",
		"It stores structured data that supports recall, retention, and historical traceability.",
		"Writers append immutable records whenever possible, and maintenance jobs compact or archive old data without losing essential meaning.",
		"Files under this path can include event logs, tiered memory facts, runtime context snapshots, and metadata state documents.",
		"The primary goal is predictable retrieval quality with bounded storage growth and clear operational observability.",
		"Readers should treat this directory as a managed area and avoid manual mutation unless performing recovery.",
		"Path: " + dir + ".",
	}, " ")

	if wordCount(text) < 100 {
		text += " This abstract intentionally includes enough context so listing calls can quickly explain folder intent without loading all underlying records."
	}

	return text
}

// buildDefaultOverview builds a deterministic default overview for a folder and returns <=2000 words.
func buildDefaultOverview(dir string) string {
	return strings.Join([]string{
		"Overview for", dir + ".",
		"This folder belongs to the managed memory layout and can contain session metadata, raw immutable event shards, compact summaries, tiered fact records, and runtime context files.",
		"Retention and cleanup are policy driven.",
		"L0 facts are durable unless explicitly removed for compliance.",
		"L1 and L2 facts can expire automatically based on UTC retention windows.",
		"Background maintenance can archive old raw shards, refresh summary files, and remove expired data while preserving read-path continuity.",
		"Consumers should prefer SDK APIs for reads and writes to maintain consistency guarantees.",
	}, " ")
}
