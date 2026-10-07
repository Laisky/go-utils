package memory

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// AfterTurn persists turn events, writes tiered facts, and updates metadata.
func (engine *StandardEngine) AfterTurn(ctx context.Context, in AfterTurnInput) error {
	if err := validateAfterTurnInput(in); err != nil {
		return errors.Wrap(err, "validate after-turn input")
	}

	conversation, err := normalizeAfterTurnConversation(in)
	if err != nil {
		return errors.Wrap(err, "normalize after-turn conversation")
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
	persistInputItems, persistedTrimCount, err := engine.resolvePersistInputItems(
		ctx,
		in,
		conversation,
	)
	if err != nil {
		return errors.Wrap(err, "prepare turn input for persistence")
	}

	events, err := engine.persistTurnEvents(ctx, in, now, nowRFC3339, persistInputItems)
	if err != nil {
		return err
	}

	activeIndex, err := engine.processTurnFacts(
		ctx,
		in,
		now,
		nowRFC3339,
		persistInputItems,
	)
	if err != nil {
		return err
	}

	if err = engine.finalizeAfterTurn(
		ctx,
		in,
		meta,
		nowRFC3339,
		events,
		activeIndex,
		persistedTrimCount,
	); err != nil {
		return err
	}

	return nil
}

// resolvePersistInputItems selects the current-turn items that should be persisted.
func (engine *StandardEngine) resolvePersistInputItems(
	ctx context.Context,
	in AfterTurnInput,
	conversation normalizedConversation,
) ([]ResponseItem, int, error) {
	persistInputItems := []ResponseItem(nil)
	persistedTrimCount := 0
	if len(in.ConversationItems) > 0 {
		persistInputItems = stripMemoryReferenceItems(conversation.CurrentItems)
		persistedTrimCount = len(conversation.AllItems) - len(persistInputItems)
	}
	if len(persistInputItems) == 0 {
		preparedItems, err := engine.prepareTurnInputForPersist(
			ctx,
			in.Project,
			in.SessionID,
			in.InputItems,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, "prepare delta input items")
		}
		persistInputItems = preparedItems
	}

	return persistInputItems, persistedTrimCount, nil
}

// persistTurnEvents appends turn events to canonical and legacy event logs.
func (engine *StandardEngine) persistTurnEvents(
	ctx context.Context,
	in AfterTurnInput,
	now time.Time,
	nowRFC3339 string,
	persistInputItems []ResponseItem,
) ([]LogEvent, error) {
	events := buildTurnEvents(
		in.TurnID,
		in.UserID,
		nowRFC3339,
		persistInputItems,
		in.OutputItems,
	)
	if len(events) == 0 {
		return nil, nil
	}

	if err := engine.appendJSONL(
		ctx,
		in.Project,
		rawLogShardPath(in.SessionID, now),
		events,
	); err != nil {
		return nil, errors.Wrap(err, "append raw log events")
	}
	if err := engine.appendJSONL(
		ctx,
		in.Project,
		runtimeContextPath(in.SessionID),
		events,
	); err != nil {
		return nil, errors.Wrap(err, "append runtime context events")
	}
	if err := engine.appendJSONL(
		ctx,
		in.Project,
		legacyLogPath(in.SessionID),
		events,
	); err != nil {
		return nil, errors.Wrap(err, "append legacy log events")
	}
	if err := engine.appendJSONL(
		ctx,
		in.Project,
		legacyContextPath(in.SessionID),
		events,
	); err != nil {
		return nil, errors.Wrap(err, "append legacy context events")
	}

	return events, nil
}

// processTurnFacts extracts facts, applies heuristic merge, and persists mutations.
func (engine *StandardEngine) processTurnFacts(
	ctx context.Context,
	in AfterTurnInput,
	now time.Time,
	nowRFC3339 string,
	persistInputItems []ResponseItem,
) (ActiveFactsIndex, error) {
	activeIndex, err := engine.loadActiveFactsIndex(ctx, in.Project, in.SessionID)
	if err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "load active facts index")
	}

	existingFacts := make([]MemoryFact, 0, len(activeIndex.Facts))
	for _, fact := range activeIndex.Facts {
		existingFacts = append(existingFacts, fact)
	}

	facts := extractFacts(in.TurnID, nowRFC3339, persistInputItems)
	deletedFactIDs := []string(nil)
	if engine.heuristic != nil {
		heuristicResult, heuristicErr := engine.heuristic.ExtractAndMergeFacts(
			ctx,
			HeuristicFactInput{
				TurnID:        in.TurnID,
				NowRFC3339:    nowRFC3339,
				UserID:        in.UserID,
				InputItems:    persistInputItems,
				ExistingFacts: existingFacts,
			},
		)
		if heuristicErr == nil {
			facts = mergeFactCandidates(facts, heuristicResult.UpdatedFacts)
			deletedFactIDs = heuristicResult.DeletedFactIDs
		}
	}

	for idx := range facts {
		facts[idx] = applyTierPolicy(now, engine.conf, facts[idx])
		facts[idx].SourceTurnID = in.TurnID
		facts[idx].SourceUserID = in.UserID
		facts[idx].State = memoryStateActive
	}

	if len(facts) == 0 && len(deletedFactIDs) == 0 {
		return activeIndex, nil
	}

	mutationPlan := selectExactFactMutations(
		now,
		in.TurnID,
		in.UserID,
		activeIndex.Facts,
		facts,
		deletedFactIDs,
	)
	if len(mutationPlan.Writes) > 0 {
		if err = engine.writeTieredFacts(
			ctx,
			in.Project,
			in.SessionID,
			now,
			mutationPlan.Writes,
		); err != nil {
			return ActiveFactsIndex{}, errors.Wrap(err, "write tiered facts")
		}
		if err = engine.appendJSONL(
			ctx,
			in.Project,
			legacyFactsPath(in.SessionID),
			mutationPlan.Writes,
		); err != nil {
			return ActiveFactsIndex{}, errors.Wrap(err, "append legacy facts")
		}
	}

	activeIndex.Facts = mutationPlan.NextActiveFacts
	if err = engine.writeActiveFactsIndex(ctx, in.Project, in.SessionID, activeIndex); err != nil {
		return ActiveFactsIndex{}, errors.Wrap(err, "write active facts index")
	}
	if metricsErr := engine.mutateMetrics(ctx, in.Project, in.SessionID, func(metrics *MemoryMetrics) {
		metrics.DedupeSkipCount += mutationPlan.DedupeSkipCount
	}); metricsErr != nil {
		// Metrics must not break persistence.
	}

	return activeIndex, nil
}

// finalizeAfterTurn updates watermarks, metrics, and metadata after persistence succeeds.
func (engine *StandardEngine) finalizeAfterTurn(
	ctx context.Context,
	in AfterTurnInput,
	meta MemoryMeta,
	nowRFC3339 string,
	events []LogEvent,
	activeIndex ActiveFactsIndex,
	persistedTrimCount int,
) error {
	watermarks, watermarkErr := engine.loadWatermarks(ctx, in.Project, in.SessionID)
	if watermarkErr == nil {
		watermarks.LastProcessedTurnID = in.TurnID
		if len(events) > 0 {
			watermarks.LastRawEventID = events[len(events)-1].ID
			watermarks.LastRawEventTS = nowRFC3339
			watermarks.RawEventCount += len(events)
			watermarks.RuntimeContextCount += len(events)
		}
		watermarks.ActiveFactCount = len(activeIndex.Facts)
		_ = engine.writeWatermarks(ctx, in.Project, in.SessionID, watermarks)
	}
	if metricsErr := engine.mutateMetrics(ctx, in.Project, in.SessionID, func(metrics *MemoryMetrics) {
		metrics.PersistedHistoryTrimCount += max(0, persistedTrimCount)
	}); metricsErr != nil {
		// Metrics must not break persistence.
	}

	meta.Version = 1
	meta.LatestTurnID = in.TurnID
	meta.ProcessedTurnIDs = append(meta.ProcessedTurnIDs, in.TurnID)
	if len(meta.ProcessedTurnIDs) > engine.conf.MaxProcessedTurns {
		meta.ProcessedTurnIDs = meta.ProcessedTurnIDs[len(meta.ProcessedTurnIDs)-engine.conf.MaxProcessedTurns:]
	}
	meta.UpdatedAt = nowRFC3339

	if err := engine.writeMeta(ctx, in.Project, in.SessionID, meta); err != nil {
		return errors.Wrap(err, "write meta")
	}

	return nil
}

// prepareTurnInputForPersist removes recalled context items and keeps only turn delta inputs for persistence.
//
// Parameters:
//   - ctx: Request-scoped context.
//   - project: Project namespace.
//   - sessionID: Session identifier.
//   - inputItems: Input items provided to AfterTurn.
//
// Returns:
//   - Input items reduced to current-turn deltas.
//   - Error when runtime context lookup fails unexpectedly.
func (engine *StandardEngine) prepareTurnInputForPersist(
	ctx context.Context,
	project, sessionID string,
	inputItems []ResponseItem,
) ([]ResponseItem, error) {
	filtered := stripMemoryReferenceItems(inputItems)

	contextEvents, loadErr := engine.loadContextEventsWithFallback(ctx, project, sessionID)
	if loadErr != nil {
		return nil, errors.Wrap(loadErr, "load context events")
	}

	recentItems := engine.pickRecentContextItems(contextEvents, engine.conf.RecentContextItems)
	if len(recentItems) == 0 || len(filtered) == 0 || len(filtered) <= len(recentItems) {
		return filtered, nil
	}

	for idx := range recentItems {
		if responseItemIdentity(filtered[idx]) != responseItemIdentity(recentItems[idx]) {
			return filtered, nil
		}
	}

	return filtered[len(recentItems):], nil
}

// stripMemoryReferenceItems removes the engine's fixed policy/data pair before
// persistence. A standalone user message is never discarded just for containing
// reference-looking text. Legacy developer recall messages remain recognized.
func stripMemoryReferenceItems(items []ResponseItem) []ResponseItem {
	filtered := make([]ResponseItem, 0, len(items))
	for i := 0; i < len(items); i++ {
		item := items[i]
		if isFixedMemoryPolicy(item) {
			if i+1 < len(items) && isMemoryReferenceData(items[i+1]) {
				i++
			}
			continue
		}
		if item.Role == responseRoleDeveloper && isMemoryReferenceItem(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// isMemoryReferenceItem recognizes the generated reference wire format for
// bookkeeping, not as proof of trusted provenance or permission to execute.
func isMemoryReferenceItem(item ResponseItem) bool {
	if isFixedMemoryPolicy(item) || isMemoryReferenceData(item) {
		return true
	}
	if item.Role != responseRoleDeveloper {
		return false
	}
	for _, part := range item.Content {
		if strings.Contains(part.Text, "<memory_reference>") && strings.Contains(part.Text, "</memory_reference>") {
			return true
		}
	}
	return false
}

// mergeFactCandidates merges rule-based and heuristic facts and keeps heuristic facts as preferred candidates.
func mergeFactCandidates(ruleFacts, heuristicFacts []MemoryFact) []MemoryFact {
	if len(heuristicFacts) == 0 {
		return ruleFacts
	}

	type key struct {
		factID string
		field  string
	}

	merged := make(map[key]MemoryFact, len(ruleFacts)+len(heuristicFacts))
	ordered := make([]key, 0, len(ruleFacts)+len(heuristicFacts))
	appendFact := func(fact MemoryFact) {
		k := key{factID: strings.TrimSpace(fact.FactID), field: strings.TrimSpace(fact.Key)}
		if k.factID == "" && k.field == "" {
			return
		}
		if _, exists := merged[k]; !exists {
			ordered = append(ordered, k)
		}
		merged[k] = fact
	}

	for _, fact := range ruleFacts {
		appendFact(fact)
	}
	for _, fact := range heuristicFacts {
		appendFact(fact)
	}

	result := make([]MemoryFact, 0, len(merged))
	for _, k := range ordered {
		result = append(result, merged[k])
	}

	return result
}
