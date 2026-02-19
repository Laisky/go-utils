package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// newStandardEngine validates config and creates a standard engine instance.
func newStandardEngine(storage files.Storage, conf Config) (*StandardEngine, error) {
	if storage == nil {
		return nil, errors.Errorf("storage is required")
	}

	if conf.RecentContextItems <= 0 {
		conf.RecentContextItems = defaultRecentContextItems
	}
	if conf.RecallFactsLimit <= 0 {
		conf.RecallFactsLimit = defaultRecallFactsLimit
	}
	if conf.SearchLimit <= 0 {
		conf.SearchLimit = defaultSearchLimit
	}
	if conf.CompactThreshold <= 0 || conf.CompactThreshold >= 1 {
		conf.CompactThreshold = defaultCompactThreshold
	}
	if conf.L1RetentionDays <= 0 {
		conf.L1RetentionDays = defaultL1RetentionDays
	}
	if conf.L2RetentionDays <= 0 {
		conf.L2RetentionDays = defaultL2RetentionDays
	}
	if conf.CompactionMinAge <= 0 {
		conf.CompactionMinAge = defaultCompactionMinAge
	}
	if conf.SummaryRefreshInterval <= 0 {
		conf.SummaryRefreshInterval = defaultSummaryRefreshInterval
	}
	if conf.MaxProcessedTurns <= 0 {
		conf.MaxProcessedTurns = defaultMaxProcessedTurns
	}
	if conf.TimeNow == nil {
		conf.TimeNow = time.Now
	}

	return &StandardEngine{storage: storage, conf: conf}, nil
}

// BeforeTurn loads context and recalls memory facts to build model input items.
func (engine *StandardEngine) BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error) {
	if err := validateBeforeTurnInput(in); err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "validate before-turn input")
	}

	contextEvents, err := engine.loadContextEventsWithFallback(ctx, in.Project, in.SessionID)
	if err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "load context events")
	}

	facts, err := engine.loadRecallFacts(ctx, in.Project, in.SessionID)
	if err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "load recall facts")
	}

	query := strings.TrimSpace(extractInputText(in.CurrentInput))
	var chunks []files.FileChunk
	if query != "" {
		chunks, err = engine.storage.Search(ctx, in.Project, query, sessionBasePath(in.SessionID), engine.conf.SearchLimit)
		if err != nil {
			chunks = nil
		}
	}

	memoryBlock, factIDs := engine.buildMemoryBlock(facts, chunks)
	recentItems := engine.pickRecentContextItems(contextEvents, engine.conf.RecentContextItems)

	items := make([]ResponseItem, 0, len(recentItems)+len(in.CurrentInput)+1)
	if memoryBlock != nil {
		items = append(items, *memoryBlock)
	}
	items = append(items, recentItems...)
	items = append(items, in.CurrentInput...)

	tokenCount := estimateTokens(items)
	if in.MaxInputTok > 0 {
		if float64(tokenCount) >= float64(in.MaxInputTok)*engine.conf.CompactThreshold {
			if compactErr := engine.compactRuntimeContext(ctx, in.Project, in.SessionID, contextEvents, in.MaxInputTok); compactErr != nil {
				// Keep serving current request even when compaction fails.
			} else {
				contextEvents, _ = engine.loadContextEventsWithFallback(ctx, in.Project, in.SessionID)
				recentItems = engine.pickRecentContextItems(contextEvents, engine.conf.RecentContextItems)
				items = items[:0]
				if memoryBlock != nil {
					items = append(items, *memoryBlock)
				}
				items = append(items, recentItems...)
				items = append(items, in.CurrentInput...)
				tokenCount = estimateTokens(items)
			}
		}
	}

	return BeforeTurnOutput{
		InputItems:        items,
		RecallFactIDs:     factIDs,
		ContextTokenCount: tokenCount,
	}, nil
}

// AfterTurn persists turn events, writes tiered facts, and updates metadata.
func (engine *StandardEngine) AfterTurn(ctx context.Context, in AfterTurnInput) error {
	if err := validateAfterTurnInput(in); err != nil {
		return errors.Wrap(err, "validate after-turn input")
	}

	if err := engine.ensureSessionScaffold(ctx, in.Project, in.SessionID); err != nil {
		return errors.Wrap(err, "ensure session scaffold")
	}

	meta, err := engine.loadMeta(ctx, in.Project, in.SessionID)
	if err != nil {
		return errors.Wrap(err, "load meta")
	}
	if contains(meta.ProcessedTurnIDs, in.TurnID) {
		return nil
	}

	now := engine.conf.TimeNow().UTC()
	nowRFC3339 := now.Format(time.RFC3339)

	events := buildTurnEvents(in.TurnID, nowRFC3339, in.InputItems, in.OutputItems)
	if len(events) > 0 {
		if err = engine.appendJSONL(ctx, in.Project, rawLogShardPath(in.SessionID, now), events); err != nil {
			return errors.Wrap(err, "append raw log events")
		}
		if err = engine.appendJSONL(ctx, in.Project, runtimeContextPath(in.SessionID), events); err != nil {
			return errors.Wrap(err, "append runtime context events")
		}

		// Keep legacy files updated during migration window.
		if err = engine.appendJSONL(ctx, in.Project, legacyLogPath(in.SessionID), events); err != nil {
			return errors.Wrap(err, "append legacy log events")
		}
		if err = engine.appendJSONL(ctx, in.Project, legacyContextPath(in.SessionID), events); err != nil {
			return errors.Wrap(err, "append legacy context events")
		}
	}

	facts := extractFacts(in.TurnID, nowRFC3339, in.InputItems)
	if len(facts) > 0 {
		for idx := range facts {
			facts[idx] = applyTierPolicy(now, engine.conf, facts[idx])
			facts[idx].SourceTurnID = in.TurnID
		}
		if err = engine.writeTieredFacts(ctx, in.Project, in.SessionID, now, facts); err != nil {
			return errors.Wrap(err, "write tiered facts")
		}
		if err = engine.appendJSONL(ctx, in.Project, legacyFactsPath(in.SessionID), facts); err != nil {
			return errors.Wrap(err, "append legacy facts")
		}
	}

	meta.Version = 1
	meta.LatestTurnID = in.TurnID
	meta.ProcessedTurnIDs = append(meta.ProcessedTurnIDs, in.TurnID)
	if len(meta.ProcessedTurnIDs) > engine.conf.MaxProcessedTurns {
		meta.ProcessedTurnIDs = meta.ProcessedTurnIDs[len(meta.ProcessedTurnIDs)-engine.conf.MaxProcessedTurns:]
	}
	meta.UpdatedAt = nowRFC3339

	if err = engine.writeMeta(ctx, in.Project, in.SessionID, meta); err != nil {
		return errors.Wrap(err, "write meta")
	}

	return nil
}

// compactRuntimeContext compacts runtime context and writes compact events when context grows too large.
func (engine *StandardEngine) compactRuntimeContext(ctx context.Context,
	project, sessionID string,
	events []LogEvent,
	maxInputTok int,
) error {
	if len(events) <= engine.conf.RecentContextItems {
		return nil
	}

	keepFrom := len(events) - engine.conf.RecentContextItems
	older := events[:keepFrom]
	recent := events[keepFrom:]
	if len(older) == 0 {
		return nil
	}

	now := engine.conf.TimeNow().UTC()
	nowRFC3339 := now.Format(time.RFC3339)
	summary := fmt.Sprintf(
		"compacted %d context events to protect context window (max_input_tok=%d)",
		len(older),
		maxInputTok,
	)
	compactEvent := LogEvent{
		ID:      "compact-" + now.Format("20060102T150405.000000000"),
		TS:      nowRFC3339,
		Type:    "compact_summary",
		Summary: summary,
	}

	newEvents := make([]LogEvent, 0, len(recent)+1)
	newEvents = append(newEvents, compactEvent)
	newEvents = append(newEvents, recent...)

	body, err := marshalJSONL(newEvents)
	if err != nil {
		return errors.Wrap(err, "marshal compacted context")
	}

	if err = engine.storage.Write(ctx, project, runtimeContextPath(sessionID), body, files.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write compacted runtime context")
	}

	if err = engine.appendJSONL(ctx, project, compactShardPath(sessionID, now), []LogEvent{compactEvent}); err != nil {
		return errors.Wrap(err, "append compact event")
	}

	pointerBody, err := json.Marshal(map[string]string{
		"last_compact_at": nowRFC3339,
		"compact_file":    compactShardPath(sessionID, now),
	})
	if err != nil {
		return errors.Wrap(err, "marshal compact pointer")
	}
	if err = engine.storage.Write(ctx, project, latestCompactPointerPath(sessionID), string(pointerBody), files.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write compact pointer")
	}

	meta, loadErr := engine.loadMeta(ctx, project, sessionID)
	if loadErr == nil {
		meta.LastCompactAt = nowRFC3339
		meta.UpdatedAt = nowRFC3339
		_ = engine.writeMeta(ctx, project, sessionID, meta)
	}

	return nil
}

// buildMemoryBlock builds one developer memory block from recalled facts and search hits.
func (engine *StandardEngine) buildMemoryBlock(facts []MemoryFact, chunks []files.FileChunk) (*ResponseItem, []string) {
	if len(facts) == 0 && len(chunks) == 0 {
		return nil, nil
	}

	factIDs := make([]string, 0, len(facts))
	lines := make([]string, 0, len(facts)+len(chunks)+1)
	lines = append(lines, "Memory recall:")
	for _, fact := range facts {
		factIDs = append(factIDs, fact.FactID)
		if strings.TrimSpace(fact.Tier) == "" {
			lines = append(lines, fmt.Sprintf(
				"- Fact[%s] %s=%s (confidence=%.2f)",
				fact.FactID,
				fact.Key,
				fact.Value,
				fact.Confidence,
			))
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"- Fact[%s][%s] %s=%s (confidence=%.2f)",
			fact.FactID,
			fact.Tier,
			fact.Key,
			fact.Value,
			fact.Confidence,
		))
	}
	for _, chunk := range chunks {
		lines = append(lines, fmt.Sprintf(
			"- Recall[%s:%d-%d] %s",
			chunk.FilePath,
			chunk.StartBytes,
			chunk.EndBytes,
			chunk.Content,
		))
	}

	item := ResponseItem{
		Type: "message",
		Role: "developer",
		Content: []ResponseContentPart{{
			Type: "input_text",
			Text: strings.Join(lines, "\n"),
		}},
	}

	return &item, factIDs
}

// pickRecentContextItems extracts recent response items from context events and returns up to maxItems.
func (engine *StandardEngine) pickRecentContextItems(events []LogEvent, maxItems int) []ResponseItem {
	items := make([]ResponseItem, 0, len(events))
	for _, event := range events {
		if event.Item.Type == "" {
			continue
		}
		items = append(items, event.Item)
	}

	if len(items) > maxItems {
		items = items[len(items)-maxItems:]
	}

	return items
}
