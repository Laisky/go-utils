package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// loadActiveFactsIndex loads the exact active-facts index or rebuilds it.
func (engine *StandardEngine) loadActiveFactsIndex(
	ctx context.Context,
	project, sessionID string,
) (ActiveFactsIndex, error) {
	info, err := engine.storage.Stat(ctx, project, activeFactsIndexPath(sessionID))
	if err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "stat active facts index")
	}
	if info.Exists && info.Type == storageengine.FileTypeFile {
		body, readErr := engine.storage.Read(ctx, project, activeFactsIndexPath(sessionID), 0, -1)
		if readErr != nil {
			return ActiveFactsIndex{}, errors.Wrap(readErr, "read active facts index")
		}
		if strings.TrimSpace(body) != "" {
			var index ActiveFactsIndex
			if unmarshalErr := json.Unmarshal([]byte(body), &index); unmarshalErr == nil {
				if index.Facts == nil {
					index.Facts = make(map[string]MemoryFact)
				}
				return index, nil
			}
		}
	}

	index, err := engine.rebuildActiveFactsIndex(ctx, project, sessionID)
	if err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "rebuild active facts index")
	}

	return index, nil
}

// rebuildActiveFactsIndex rebuilds the exact active-facts index.
func (engine *StandardEngine) rebuildActiveFactsIndex(
	ctx context.Context,
	project, sessionID string,
) (ActiveFactsIndex, error) {
	facts, err := engine.loadAllFacts(ctx, project, sessionID)
	if err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "load all facts")
	}

	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].TS == facts[j].TS {
			return facts[i].ID < facts[j].ID
		}
		return facts[i].TS < facts[j].TS
	})

	active := make(map[string]MemoryFact)
	now := engine.conf.TimeNow().UTC()
	for _, fact := range facts {
		identity := factIdentity(fact)
		if identity == "" {
			continue
		}
		if isFactExpired(now, fact) {
			continue
		}

		switch normalizeMemoryState(fact.State, fact.Type) {
		case memoryStateDeleted, memoryStateSuperseded, memoryStateExpired, memoryStateContradicted:
			delete(active, identity)
		default:
			active[identity] = fact
		}
	}

	index := ActiveFactsIndex{
		UpdatedAt: engine.conf.TimeNow().UTC().Format(time.RFC3339),
		Facts:     active,
	}
	if err = engine.writeActiveFactsIndex(ctx, project, sessionID, index); err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "write rebuilt active facts index")
	}

	return index, nil
}

// writeActiveFactsIndex persists the exact active-facts index.
func (engine *StandardEngine) writeActiveFactsIndex(
	ctx context.Context,
	project, sessionID string,
	index ActiveFactsIndex,
) error {
	if index.Facts == nil {
		index.Facts = make(map[string]MemoryFact)
	}
	index.UpdatedAt = engine.conf.TimeNow().UTC().Format(time.RFC3339)
	body, err := json.Marshal(index)
	if err != nil {
		return errors.Wrap(err, "marshal active facts index")
	}

	if err = engine.storage.Write(
		ctx,
		project,
		activeFactsIndexPath(sessionID),
		string(body),
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
		return errors.Wrap(err, "write active facts index")
	}

	return nil
}

// loadAllFacts loads every stored fact record across canonical and legacy paths.
func (engine *StandardEngine) loadAllFacts(ctx context.Context, project, sessionID string) ([]MemoryFact, error) {
	allFacts := make([]MemoryFact, 0, 64)
	for _, tier := range []string{memoryTierL0, memoryTierL1, memoryTierL2} {
		tierFacts, err := engine.loadTierFacts(ctx, project, sessionID, tier)
		if err != nil {
			return nil, errors.Wrapf(err, "load tier facts %s", tier)
		}
		allFacts = append(allFacts, tierFacts...)
	}

	legacyFacts, err := engine.loadFactsFromFile(ctx, project, legacyFactsPath(sessionID))
	if err != nil {
		return nil, errors.Wrap(err, "load legacy facts")
	}
	allFacts = append(allFacts, legacyFacts...)

	return allFacts, nil
}

// loadWatermarks loads maintenance watermarks and returns an empty default when missing.
func (engine *StandardEngine) loadWatermarks(ctx context.Context, project, sessionID string) (MemoryWatermarks, error) {
	info, err := engine.storage.Stat(ctx, project, metaWatermarksPath(sessionID))
	if err != nil {
		return MemoryWatermarks{}, errors.Wrap(err, "stat watermarks")
	}
	if !info.Exists || info.Type != storageengine.FileTypeFile {
		return MemoryWatermarks{}, nil
	}

	body, err := engine.storage.Read(ctx, project, metaWatermarksPath(sessionID), 0, -1)
	if err != nil {
		return MemoryWatermarks{}, errors.Wrap(err, "read watermarks")
	}
	if strings.TrimSpace(body) == "" {
		return MemoryWatermarks{}, nil
	}

	var watermarks MemoryWatermarks
	if err = json.Unmarshal([]byte(body), &watermarks); err != nil {
		return MemoryWatermarks{}, errors.Wrap(err, "unmarshal watermarks")
	}

	return watermarks, nil
}

// writeWatermarks persists maintenance watermarks.
func (engine *StandardEngine) writeWatermarks(
	ctx context.Context,
	project, sessionID string,
	watermarks MemoryWatermarks,
) error {
	watermarks.UpdatedAt = engine.conf.TimeNow().UTC().Format(time.RFC3339)
	body, err := json.Marshal(watermarks)
	if err != nil {
		return errors.Wrap(err, "marshal watermarks")
	}

	if err = engine.storage.Write(
		ctx,
		project,
		metaWatermarksPath(sessionID),
		string(body),
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
		return errors.Wrap(err, "write watermarks")
	}

	return nil
}

// loadMetrics loads V2 memory metrics and returns zero values when missing.
func (engine *StandardEngine) loadMetrics(ctx context.Context, project, sessionID string) (MemoryMetrics, error) {
	info, err := engine.storage.Stat(ctx, project, metaMetricsPath(sessionID))
	if err != nil {
		return MemoryMetrics{}, errors.Wrap(err, "stat metrics")
	}
	if !info.Exists || info.Type != storageengine.FileTypeFile {
		return MemoryMetrics{}, nil
	}

	body, err := engine.storage.Read(ctx, project, metaMetricsPath(sessionID), 0, -1)
	if err != nil {
		return MemoryMetrics{}, errors.Wrap(err, "read metrics")
	}
	if strings.TrimSpace(body) == "" {
		return MemoryMetrics{}, nil
	}

	var metrics MemoryMetrics
	if err = json.Unmarshal([]byte(body), &metrics); err != nil {
		return MemoryMetrics{}, errors.Wrap(err, "unmarshal metrics")
	}

	return metrics, nil
}

// writeMetrics persists V2 memory metrics.
func (engine *StandardEngine) writeMetrics(
	ctx context.Context,
	project, sessionID string,
	metrics MemoryMetrics,
) error {
	metrics.UpdatedAt = engine.conf.TimeNow().UTC().Format(time.RFC3339)
	body, err := json.Marshal(metrics)
	if err != nil {
		return errors.Wrap(err, "marshal metrics")
	}

	if err = engine.storage.Write(
		ctx,
		project,
		metaMetricsPath(sessionID),
		string(body),
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
		return errors.Wrap(err, "write metrics")
	}

	return nil
}

// mutateMetrics loads, updates, and rewrites metrics in one helper.
func (engine *StandardEngine) mutateMetrics(
	ctx context.Context,
	project, sessionID string,
	mutate func(*MemoryMetrics),
) error {
	metrics, err := engine.loadMetrics(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load metrics")
	}
	mutate(&metrics)
	if err = engine.writeMetrics(ctx, project, sessionID, metrics); err != nil {
		return errors.Wrap(err, "write metrics")
	}

	return nil
}

// loadInsights loads all insight records for one session.
func (engine *StandardEngine) loadInsights(ctx context.Context, project, sessionID string) ([]InsightRecord, error) {
	fileInfos, err := engine.listFiles(ctx, project, insightsRootPath(sessionID), ".jsonl")
	if err != nil {
		return nil, errors.Wrap(err, "list insights")
	}

	insights := make([]InsightRecord, 0, len(fileInfos)*4)
	for _, info := range fileInfos {
		records, readErr := engine.loadJSONL(ctx, project, info.Path)
		if readErr != nil {
			return nil, errors.Wrapf(readErr, "load insight file %s", info.Path)
		}
		for _, line := range records {
			var insight InsightRecord
			if unmarshalErr := json.Unmarshal([]byte(line), &insight); unmarshalErr != nil {
				continue
			}
			insights = append(insights, insight)
		}
	}

	return insights, nil
}

// loadRecallInsights ranks and bounds insight recall for the current query.
func (engine *StandardEngine) loadRecallInsights(
	ctx context.Context,
	project, sessionID, query string,
) ([]InsightRecord, error) {
	insights, err := engine.loadInsights(ctx, project, sessionID)
	if err != nil {
		return nil, errors.Wrap(err, "load insights")
	}
	if len(insights) == 0 {
		return nil, nil
	}

	type rankedInsight struct {
		record InsightRecord
		score  float64
	}
	terms := tokenizeRecallQuery(query)
	ranked := make([]rankedInsight, 0, len(insights))
	for _, insight := range insights {
		score := insight.Confidence
		haystack := strings.ToLower(strings.TrimSpace(
			insight.Type + " " + insight.Summary + " " + strings.Join(insight.RelatedFactIDs, " "),
		))
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				score += 0.3
			}
		}
		ranked = append(ranked, rankedInsight{record: insight, score: score})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].record.TS > ranked[j].record.TS
		}
		return ranked[i].score > ranked[j].score
	})

	limit := engine.conf.InsightRecallLimit
	if limit <= 0 {
		limit = defaultInsightRecallLimit
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	result := make([]InsightRecord, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.record)
	}

	return result, nil
}

// appendInsights appends insight records to the daily insight shard.
func (engine *StandardEngine) appendInsights(
	ctx context.Context,
	project, sessionID string,
	now time.Time,
	insights []InsightRecord,
) error {
	if len(insights) == 0 {
		return nil
	}

	lines := make([]string, 0, len(insights))
	for _, insight := range insights {
		buf, err := json.Marshal(insight)
		if err != nil {
			return errors.Wrap(err, "marshal insight")
		}
		lines = append(lines, string(buf))
	}

	body := strings.Join(lines, "\n") + "\n"
	if err := engine.storage.Write(
		ctx,
		project,
		insightsShardPath(sessionID, now),
		body,
		storageengine.WriteModeAppend,
		0,
	); err != nil {
		return errors.Wrap(err, "append insights")
	}

	return nil
}

// normalizeMemoryState normalizes state and operation type into one effective state.
func normalizeMemoryState(state, recordType string) string {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized != "" {
		return normalized
	}

	switch strings.TrimSpace(strings.ToLower(recordType)) {
	case "fact_delete":
		return memoryStateDeleted
	case "fact_supersede":
		return memoryStateSuperseded
	default:
		return memoryStateActive
	}
}
