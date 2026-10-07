package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// newStandardEngine validates config and creates a standard engine instance.
func newStandardEngine(storage storageengine.Engine, conf Config) (*StandardEngine, error) {
	if storage == nil {
		return nil, errors.Errorf("storage is required")
	}

	if conf.RecentContextItems <= 0 {
		conf.RecentContextItems = defaultRecentContextItems
	}
	if conf.RecallFactsLimit <= 0 {
		conf.RecallFactsLimit = defaultRecallFactsLimit
	}
	if conf.InsightRecallLimit <= 0 {
		conf.InsightRecallLimit = defaultInsightRecallLimit
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
	if conf.ConsolidationMinEvents <= 0 {
		conf.ConsolidationMinEvents = defaultConsolidationMinEvents
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
	if strings.TrimSpace(conf.LLMModel) == "" {
		conf.LLMModel = defaultLLMModel
	}
	if conf.LLMTimeout <= 0 {
		conf.LLMTimeout = defaultLLMTimeout
	}
	if conf.LLMMaxOutputTokens <= 0 {
		conf.LLMMaxOutputTokens = defaultLLMMaxOutputTokens
	}
	if conf.TimeNow == nil {
		conf.TimeNow = time.Now
	}

	heuristic := conf.HeuristicClient
	if heuristic == nil && strings.TrimSpace(conf.LLMAPIBase) != "" && strings.TrimSpace(conf.LLMAPIKey) != "" {
		llmClient, buildErr := newOpenAIResponsesClient(openAIResponsesClientConfig{
			APIBase:         conf.LLMAPIBase,
			APIKey:          conf.LLMAPIKey,
			Model:           conf.LLMModel,
			Timeout:         conf.LLMTimeout,
			MaxOutputTokens: conf.LLMMaxOutputTokens,
			// Cleartext http is only accepted through the explicit opt-in.
			AllowInsecureHTTP: conf.LLMAllowInsecureHTTP,
		})
		if buildErr != nil {
			return nil, errors.Wrap(buildErr, "build heuristic client")
		}
		// Assign to the outer variable: a ":=" here once shadowed it and left
		// the engine without the configured heuristic client.
		heuristic = llmClient
		conf.HeuristicClient = llmClient
	}

	return &StandardEngine{storage: storage, heuristic: heuristic, conf: conf}, nil
}

// BeforeTurn loads context and recalls memory facts to build model input items.
func (engine *StandardEngine) BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error) {
	if err := validateBeforeTurnInput(in); err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "validate before-turn input")
	}

	conversation, convErr := normalizeBeforeTurnConversation(in)
	if convErr != nil {
		return BeforeTurnOutput{}, errors.Wrap(convErr, "normalize before-turn conversation")
	}

	contextEvents, contextErr := engine.loadContextEventsWithFallback(ctx, in.Project, in.SessionID)
	if contextErr != nil {
		return BeforeTurnOutput{}, errors.Wrap(contextErr, "load context events")
	}

	query := strings.TrimSpace(extractInputText(conversation.CurrentItems))

	facts, factsErr := engine.loadRecallFacts(ctx, in.Project, in.SessionID, query)
	if factsErr != nil {
		return BeforeTurnOutput{}, errors.Wrap(factsErr, "load recall facts")
	}
	insights, insightsErr := engine.loadRecallInsights(ctx, in.Project, in.SessionID, query)
	if insightsErr != nil {
		return BeforeTurnOutput{}, errors.Wrap(insightsErr, "load recall insights")
	}

	var chunks []storageengine.FileChunk
	if query != "" {
		foundChunks, searchErr := engine.storage.Search(
			ctx,
			in.Project,
			query,
			sessionBasePath(in.SessionID),
			engine.conf.SearchLimit,
		)
		if searchErr == nil {
			chunks = foundChunks
		}
	}

	memoryBlock, factIDs, insightIDs, blockErr := engine.buildMemoryBlock(facts, insights, chunks)
	if blockErr != nil {
		return BeforeTurnOutput{}, errors.Wrap(blockErr, "build memory reference")
	}
	recentItems := engine.pickRecentContextItems(contextEvents, engine.conf.RecentContextItems)
	excluded := mergeIdentitySets(conversation.HistoryIDs, conversation.CurrentIDs)
	recentItems, droppedRecent := filterItemsByIdentity(recentItems, excluded)

	items := make([]ResponseItem, 0)
	if memoryBlock != nil {
		items = append(items, memoryReferencePolicy(), *memoryBlock)
	}
	items = append(items, conversation.HistoryItems...)
	items = append(items, recentItems...)
	items = append(items, conversation.CurrentItems...)

	tokenCount := estimateTokens(items)
	if in.MaxInputTok > 0 {
		if float64(tokenCount) >= float64(in.MaxInputTok)*engine.conf.CompactThreshold {
			if refreshedContext, ok := engine.compactBeforeTurnContext(
				ctx,
				in.Project,
				in.SessionID,
				contextEvents,
				in.MaxInputTok,
			); ok {
				contextEvents = refreshedContext
				recentItems = engine.pickRecentContextItems(
					contextEvents,
					engine.conf.RecentContextItems,
				)
				recentItems, _ = filterItemsByIdentity(recentItems, excluded)
				items = items[:0]
				if memoryBlock != nil {
					items = append(items, memoryReferencePolicy(), *memoryBlock)
				}
				items = append(items, conversation.HistoryItems...)
				items = append(items, recentItems...)
				items = append(items, conversation.CurrentItems...)
				tokenCount = estimateTokens(items)
			}
		}
	}

	if metricsErr := engine.mutateMetrics(ctx, in.Project, in.SessionID, func(metrics *MemoryMetrics) {
		metrics.RecallCount += len(factIDs)
		metrics.InsightRecallCount += len(insightIDs)
		metrics.PromptDuplicateDropCount += droppedRecent
	}); metricsErr != nil {
		// Metrics must not break the hot path.
	}

	return BeforeTurnOutput{
		InputItems:        items,
		RecallFactIDs:     factIDs,
		RecallInsightIDs:  insightIDs,
		ContextTokenCount: tokenCount,
	}, nil
}

// compactBeforeTurnContext compacts runtime context and reloads it on success.
func (engine *StandardEngine) compactBeforeTurnContext(
	ctx context.Context,
	project, sessionID string,
	contextEvents []LogEvent,
	maxInputTok int,
) ([]LogEvent, bool) {
	if err := engine.compactRuntimeContext(
		ctx,
		project,
		sessionID,
		contextEvents,
		maxInputTok,
	); err != nil {
		return nil, false
	}

	refreshedContext, err := engine.loadContextEventsWithFallback(ctx, project, sessionID)
	if err != nil {
		return nil, false
	}

	return refreshedContext, true
}

// compactRuntimeContext compacts runtime context and writes compact events when context grows too large.
func (engine *StandardEngine) compactRuntimeContext(
	ctx context.Context,
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

	if err = engine.storage.Write(
		ctx,
		project,
		runtimeContextPath(sessionID),
		body,
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
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
	if err = engine.storage.Write(
		ctx,
		project,
		latestCompactPointerPath(sessionID),
		string(pointerBody),
		storageengine.WriteModeTruncate,
		0,
	); err != nil {
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
