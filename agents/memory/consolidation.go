package memory

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// RunConsolidation derives observations from raw events and fact history to write bounded insight records.
func (engine *StandardEngine) RunConsolidation(ctx context.Context, project, sessionID string) error {
	if strings.TrimSpace(project) == "" {
		return errors.Errorf("project is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.Errorf("session_id is required")
	}

	rawEvents, err := engine.loadRawEvents(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load raw events")
	}
	if len(rawEvents) < engine.conf.ConsolidationMinEvents {
		return nil
	}

	facts, err := engine.loadAllFacts(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load facts for consolidation")
	}
	if len(facts) == 0 {
		return nil
	}

	existingInsights, err := engine.loadInsights(ctx, project, sessionID)
	if err != nil {
		return errors.Wrap(err, "load existing insights")
	}
	existingIDs := make(map[string]struct{}, len(existingInsights))
	for _, insight := range existingInsights {
		existingIDs[insight.ID] = struct{}{}
	}

	observedTurns := mapTurnTextByID(rawEvents)
	insights := deriveInsightsFromFacts(engine.conf.TimeNow().UTC(), facts, observedTurns)
	newInsights := make([]InsightRecord, 0, len(insights))
	for _, insight := range insights {
		if _, exists := existingIDs[insight.ID]; exists {
			continue
		}
		newInsights = append(newInsights, insight)
	}
	if len(newInsights) == 0 {
		return nil
	}

	now := engine.conf.TimeNow().UTC()
	if err = engine.appendInsights(ctx, project, sessionID, now, newInsights); err != nil {
		return errors.Wrap(err, "append insights")
	}

	watermarks, err := engine.loadWatermarks(ctx, project, sessionID)
	if err == nil {
		watermarks.LastConsolidatedAt = now.Format(time.RFC3339)
		watermarks.LastConsolidatedEvent = rawEvents[len(rawEvents)-1].ID
		watermarks.InsightCount += len(newInsights)
		_ = engine.writeWatermarks(ctx, project, sessionID, watermarks)
	}
	if metricsErr := engine.mutateMetrics(ctx, project, sessionID, func(metrics *MemoryMetrics) {
		metrics.ConsolidationRunCount++
		metrics.ConsolidationLagEvents = len(rawEvents)
	}); metricsErr != nil {
		// Metrics must not break consolidation.
	}

	return nil
}

// loadRawEvents loads raw log events from canonical shards with legacy fallback.
func (engine *StandardEngine) loadRawEvents(ctx context.Context, project, sessionID string) ([]LogEvent, error) {
	fileInfos, err := engine.listFiles(ctx, project, eventsRawRootPath(sessionID), ".jsonl")
	if err != nil {
		return nil, errors.Wrap(err, "list raw event shards")
	}

	events := make([]LogEvent, 0, len(fileInfos)*8)
	for _, info := range fileInfos {
		shardEvents, readErr := engine.loadContextEvents(ctx, project, info.Path)
		if readErr != nil {
			return nil, errors.Wrapf(readErr, "load raw event shard %s", info.Path)
		}
		events = append(events, shardEvents...)
	}
	if len(events) == 0 {
		events, err = engine.loadContextEvents(ctx, project, legacyLogPath(sessionID))
		if err != nil {
			return nil, errors.Wrap(err, "load legacy raw log")
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].TS == events[j].TS {
			return events[i].ID < events[j].ID
		}
		return events[i].TS < events[j].TS
	})

	return events, nil
}

// mapTurnTextByID builds one turn-id to summarized raw-text map from input events.
func mapTurnTextByID(events []LogEvent) map[string]string {
	turnText := make(map[string][]string)
	for _, event := range events {
		if event.Type != "input_item" && event.Type != "output_item" {
			continue
		}
		text := strings.TrimSpace(extractInputText([]ResponseItem{event.Item}))
		if text == "" {
			continue
		}
		turnText[event.TurnID] = append(turnText[event.TurnID], text)
	}

	joined := make(map[string]string, len(turnText))
	for turnID, parts := range turnText {
		joined[turnID] = strings.Join(parts, "\n")
	}

	return joined
}

// deriveInsightsFromFacts synthesizes bounded insight records from fact history and observed turn text.
func deriveInsightsFromFacts(now time.Time, facts []MemoryFact, observedTurns map[string]string) []InsightRecord {
	type factGroup struct {
		Facts        []MemoryFact
		DistinctVals []string
		Latest       MemoryFact
	}

	groups := make(map[string]*factGroup)
	for _, fact := range facts {
		identity := factIdentity(fact)
		if identity == "" {
			continue
		}
		group, exists := groups[identity]
		if !exists {
			group = &factGroup{}
			groups[identity] = group
		}
		group.Facts = append(group.Facts, fact)
		if group.Latest.TS <= fact.TS {
			group.Latest = fact
		}
	}

	insights := make([]InsightRecord, 0, len(groups))
	for identity, group := range groups {
		valueSet := make(map[string]struct{})
		relatedTurns := make([]string, 0, len(group.Facts))
		relatedFactIDs := make([]string, 0, len(group.Facts))
		for _, fact := range group.Facts {
			normalized := normalizeFactValue(fact.Value)
			if _, exists := valueSet[normalized]; !exists {
				valueSet[normalized] = struct{}{}
				group.DistinctVals = append(group.DistinctVals, fact.Value)
			}
			relatedFactIDs = append(relatedFactIDs, fact.ID)
			if strings.TrimSpace(fact.SourceTurnID) != "" {
				relatedTurns = append(relatedTurns, fact.SourceTurnID)
			}
		}

		insightType := "fact_stability"
		status := memoryStateConsolidated
		confidence := 0.78
		var summary string
		if len(group.DistinctVals) > 1 {
			insightType = "fact_evolution"
			summary = fmt.Sprintf(
				"Memory for %s evolved across turns and currently resolves to %s=%s.",
				identity,
				group.Latest.Key,
				group.Latest.Value,
			)
			confidence = 0.84
		} else if len(group.Facts) >= 2 {
			summary = fmt.Sprintf(
				"Memory for %s remained stable across multiple turns: %s=%s.",
				identity,
				group.Latest.Key,
				group.Latest.Value,
			)
		} else {
			continue
		}

		if observed := strings.TrimSpace(observedTurns[group.Latest.SourceTurnID]); observed != "" {
			summary += " Latest supporting turn: " + truncateRunes(observed, 140)
		}

		insightID := buildInsightID(identity, insightType, group.Latest.TS, summary)
		insights = append(insights, InsightRecord{
			ID:             insightID,
			TS:             now.Format(time.RFC3339),
			Type:           insightType,
			Status:         status,
			Summary:        summary,
			Confidence:     confidence,
			RelatedFactIDs: deduplicateStrings(relatedFactIDs),
			RelatedTurnIDs: deduplicateStrings(relatedTurns),
		})
	}

	sort.SliceStable(insights, func(i, j int) bool {
		if insights[i].Type == insights[j].Type {
			return insights[i].Summary < insights[j].Summary
		}
		return insights[i].Type < insights[j].Type
	})

	return insights
}

// buildInsightID builds one deterministic insight identifier.
func buildInsightID(identity, insightType, ts, summary string) string {
	payload, err := json.Marshal(map[string]string{
		"identity": identity,
		"type":     insightType,
		"ts":       ts,
		"summary":  summary,
	})
	if err != nil {
		payload = []byte(identity + ":" + insightType + ":" + ts + ":" + summary)
	}
	sum := sha1.Sum(payload)
	return fmt.Sprintf("insight-%x", sum[:8])
}
