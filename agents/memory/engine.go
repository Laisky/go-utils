package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/agents/files"
)

const (
	logFileName         = "log.jsonl"
	contextFileName     = "context.jsonl"
	memoryFactsFileName = "memory_facts.jsonl"
	metaFileName        = "meta.json"
)

// MemoryMeta stores session-level memory bookkeeping.
type MemoryMeta struct {
	Version       int      `json:"version"`
	LatestTurnID  string   `json:"latest_turn_id,omitempty"`
	ProcessedTurn []string `json:"processed_turn_ids,omitempty"`
	LastCompactAt string   `json:"last_compact_at,omitempty"`
	UpdatedAt     string   `json:"updated_at"`
}

// LogEvent is an immutable history event stored in log/context files.
type LogEvent struct {
	ID      string       `json:"id"`
	TS      string       `json:"ts"`
	Type    string       `json:"type"`
	TurnID  string       `json:"turn_id,omitempty"`
	Item    ResponseItem `json:"item,omitempty"`
	Summary string       `json:"summary,omitempty"`
}

// MemoryFact is one structured long-term memory fact record.
type MemoryFact struct {
	ID         string  `json:"id"`
	TS         string  `json:"ts"`
	Type       string  `json:"type"`
	FactID     string  `json:"fact_id"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// newStandardEngine validates config and creates engine instance.
func newStandardEngine(storage files.Storage, conf Config) (*StandardEngine, error) {
	if storage == nil {
		return nil, errors.Errorf("storage is required")
	}

	if conf.RecentContextItems <= 0 {
		conf.RecentContextItems = 30
	}
	if conf.RecallFactsLimit <= 0 {
		conf.RecallFactsLimit = 20
	}
	if conf.SearchLimit <= 0 {
		conf.SearchLimit = 5
	}
	if conf.CompactThreshold <= 0 || conf.CompactThreshold >= 1 {
		conf.CompactThreshold = 0.8
	}
	if conf.TimeNow == nil {
		conf.TimeNow = time.Now
	}

	return &StandardEngine{storage: storage, conf: conf}, nil
}

// BeforeTurn loads context and recalls memory to build model input items.
func (engine *StandardEngine) BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error) {
	if err := validateBeforeTurnInput(in); err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "validate input")
	}

	basePath := sessionBasePath(in.SessionID)
	ctxPath := basePath + "/" + contextFileName
	factPath := basePath + "/" + memoryFactsFileName

	contextEvents, err := engine.loadContextEvents(ctx, in.Project, ctxPath)
	if err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "load context")
	}

	facts, err := engine.loadFacts(ctx, in.Project, factPath)
	if err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "load facts")
	}

	query := strings.TrimSpace(extractInputText(in.CurrentInput))
	chunks, err := engine.storage.Search(ctx, in.Project, query, basePath, engine.conf.SearchLimit)
	if err != nil {
		chunks = nil
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
			if compactErr := engine.compactContext(ctx,
				in.Project, in.SessionID, contextEvents, in.MaxInputTok); compactErr == nil {
				contextEvents, _ = engine.loadContextEvents(ctx, in.Project, ctxPath)
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

// AfterTurn persists immutable turn events and extracts structured memory facts.
func (engine *StandardEngine) AfterTurn(ctx context.Context, in AfterTurnInput) error {
	if err := validateAfterTurnInput(in); err != nil {
		return errors.Wrap(err, "validate input")
	}

	basePath := sessionBasePath(in.SessionID)
	metaPath := basePath + "/" + metaFileName
	logPath := basePath + "/" + logFileName
	ctxPath := basePath + "/" + contextFileName
	factPath := basePath + "/" + memoryFactsFileName

	meta, err := engine.loadMeta(ctx, in.Project, metaPath)
	if err != nil {
		return errors.Wrap(err, "load meta")
	}
	if contains(meta.ProcessedTurn, in.TurnID) {
		return nil
	}

	now := engine.conf.TimeNow().UTC().Format(time.RFC3339)
	logEvents := buildTurnEvents(in.TurnID, now, in.InputItems, in.OutputItems)
	if len(logEvents) > 0 {
		if err = engine.appendJSONL(ctx, in.Project, logPath, logEvents); err != nil {
			return errors.Wrap(err, "append log events")
		}
		if err = engine.appendJSONL(ctx, in.Project, ctxPath, logEvents); err != nil {
			return errors.Wrap(err, "append context events")
		}
	}

	facts := extractFacts(in.TurnID, now, in.InputItems)
	if len(facts) > 0 {
		if err = engine.appendJSONL(ctx, in.Project, factPath, facts); err != nil {
			return errors.Wrap(err, "append facts")
		}
	}

	meta.Version = 1
	meta.LatestTurnID = in.TurnID
	meta.ProcessedTurn = append(meta.ProcessedTurn, in.TurnID)
	if len(meta.ProcessedTurn) > 1024 {
		meta.ProcessedTurn = meta.ProcessedTurn[len(meta.ProcessedTurn)-1024:]
	}
	meta.UpdatedAt = now

	metaBody, err := json.Marshal(meta)
	if err != nil {
		return errors.Wrap(err, "marshal meta")
	}

	if err = engine.storage.Write(ctx,
		in.Project, metaPath, string(metaBody), files.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write meta")
	}

	return nil
}

// compactContext writes a compact event and keeps only recent context events.
func (engine *StandardEngine) compactContext(ctx context.Context,
	project, sessionID string, events []LogEvent, maxInputTok int) error {
	if len(events) <= engine.conf.RecentContextItems {
		return nil
	}

	keepFrom := len(events) - engine.conf.RecentContextItems
	older := events[:keepFrom]
	recent := events[keepFrom:]
	if len(older) == 0 {
		return nil
	}

	now := engine.conf.TimeNow().UTC().Format(time.RFC3339)
	summary := fmt.Sprintf("compacted %d events to protect context window (max_input_tok=%d)", len(older), maxInputTok)
	compactEvent := LogEvent{
		ID:      "compact-" + now,
		TS:      now,
		Type:    "compact",
		Summary: summary,
	}

	newEvents := make([]LogEvent, 0, len(recent)+1)
	newEvents = append(newEvents, compactEvent)
	newEvents = append(newEvents, recent...)

	body, err := marshalJSONL(newEvents)
	if err != nil {
		return errors.Wrap(err, "marshal compacted context")
	}

	ctxPath := sessionBasePath(sessionID) + "/" + contextFileName
	if err = engine.storage.Write(ctx, project, ctxPath, body, files.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "truncate write context")
	}

	metaPath := sessionBasePath(sessionID) + "/" + metaFileName
	meta, loadErr := engine.loadMeta(ctx, project, metaPath)
	if loadErr == nil {
		meta.LastCompactAt = now
		meta.UpdatedAt = now
		metaBody, marshalErr := json.Marshal(meta)
		if marshalErr == nil {
			_ = engine.storage.Write(ctx, project, metaPath, string(metaBody), files.WriteModeTruncate, 0)
		}
	}

	return nil
}

// appendJSONL marshals records as JSONL and appends into storage path.
func (engine *StandardEngine) appendJSONL(ctx context.Context, project, path string, records any) error {
	body, err := marshalJSONL(records)
	if err != nil {
		return errors.Wrap(err, "marshal jsonl")
	}
	if body == "" {
		return nil
	}

	if err = engine.storage.Write(ctx, project, path, body, files.WriteModeAppend, 0); err != nil {
		return errors.Wrap(err, "append jsonl")
	}

	return nil
}

// loadMeta loads session meta file and returns default meta when not found.
func (engine *StandardEngine) loadMeta(ctx context.Context, project, path string) (MemoryMeta, error) {
	info, err := engine.storage.Stat(ctx, project, path)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "stat meta")
	}
	if !info.Exists || info.Type != files.FileTypeFile {
		return MemoryMeta{Version: 1}, nil
	}

	content, err := engine.storage.Read(ctx, project, path, 0, -1)
	if err != nil {
		return MemoryMeta{}, errors.Wrap(err, "read meta")
	}
	if strings.TrimSpace(content) == "" {
		return MemoryMeta{Version: 1}, nil
	}

	var meta MemoryMeta
	if err = json.Unmarshal([]byte(content), &meta); err != nil {
		return MemoryMeta{}, errors.Wrap(err, "unmarshal meta")
	}

	if meta.Version == 0 {
		meta.Version = 1
	}

	return meta, nil
}

// loadContextEvents reads and decodes context JSONL file.
func (engine *StandardEngine) loadContextEvents(ctx context.Context, project, path string) ([]LogEvent, error) {
	records, err := engine.loadJSONL(ctx, project, path)
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

// loadFacts reads and decodes memory facts JSONL file.
func (engine *StandardEngine) loadFacts(ctx context.Context, project, path string) ([]MemoryFact, error) {
	records, err := engine.loadJSONL(ctx, project, path)
	if err != nil {
		return nil, errors.Wrap(err, "load jsonl")
	}

	facts := make([]MemoryFact, 0, len(records))
	for _, line := range records {
		var fact MemoryFact
		if err = json.Unmarshal([]byte(line), &fact); err != nil {
			continue
		}
		facts = append(facts, fact)
	}

	sort.SliceStable(facts, func(i, j int) bool {
		return facts[i].TS > facts[j].TS
	})

	if len(facts) > engine.conf.RecallFactsLimit {
		facts = facts[:engine.conf.RecallFactsLimit]
	}

	return facts, nil
}

// loadJSONL returns non-empty lines from JSONL file and handles missing files.
func (engine *StandardEngine) loadJSONL(ctx context.Context, project, path string) ([]string, error) {
	info, err := engine.storage.Stat(ctx, project, path)
	if err != nil {
		return nil, errors.Wrap(err, "stat file")
	}
	if !info.Exists || info.Type != files.FileTypeFile {
		return nil, nil
	}

	body, err := engine.storage.Read(ctx, project, path, 0, -1)
	if err != nil {
		return nil, errors.Wrap(err, "read file")
	}

	lines := make([]string, 0)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return lines, nil
}

// buildMemoryBlock builds one developer memory block from facts and search hits.
func (engine *StandardEngine) buildMemoryBlock(facts []MemoryFact, chunks []files.FileChunk) (*ResponseItem, []string) {
	if len(facts) == 0 && len(chunks) == 0 {
		return nil, nil
	}

	factIDs := make([]string, 0, len(facts))
	lines := make([]string, 0, len(facts)+len(chunks)+1)
	lines = append(lines, "Memory recall:")
	for _, fact := range facts {
		factIDs = append(factIDs, fact.FactID)
		lines = append(lines, fmt.Sprintf("- Fact[%s] %s=%s (confidence=%.2f)",
			fact.FactID, fact.Key, fact.Value, fact.Confidence))
	}
	for _, chunk := range chunks {
		lines = append(lines, fmt.Sprintf("- Recall[%s:%d-%d] %s",
			chunk.FilePath, chunk.StartBytes, chunk.EndBytes, chunk.Content))
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

// pickRecentContextItems extracts recent message items from context events.
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

// extractInputText flattens current input text for recall query.
func extractInputText(items []ResponseItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
		if strings.TrimSpace(item.Output) != "" {
			parts = append(parts, item.Output)
		}
	}

	return strings.Join(parts, "\n")
}

// estimateTokens estimates token count using a conservative rune heuristic.
func estimateTokens(items []ResponseItem) int {
	totalChars := 0
	for _, item := range items {
		totalChars += len(item.Type) + len(item.Role) + len(item.CallID) + len(item.Output)
		for _, part := range item.Content {
			totalChars += len(part.Type) + len(part.Text) + len(part.ImageURL) + len(part.FileID) + len(part.Filename)
		}
	}

	tokens := totalChars / 4
	if tokens == 0 && totalChars > 0 {
		tokens = 1
	}

	return tokens
}

// buildTurnEvents creates append-only immutable events for one conversation turn.
func buildTurnEvents(turnID, ts string, inputItems, outputItems []ResponseItem) []LogEvent {
	events := make([]LogEvent, 0, len(inputItems)+len(outputItems))
	for idx, item := range inputItems {
		events = append(events, LogEvent{
			ID:     fmt.Sprintf("%s-in-%d", turnID, idx),
			TS:     ts,
			Type:   "input_item",
			TurnID: turnID,
			Item:   item,
		})
	}
	for idx, item := range outputItems {
		events = append(events, LogEvent{
			ID:     fmt.Sprintf("%s-out-%d", turnID, idx),
			TS:     ts,
			Type:   "output_item",
			TurnID: turnID,
			Item:   item,
		})
	}

	return events
}

// extractFacts derives simple long-term memory facts from user textual input.
func extractFacts(turnID, ts string, inputItems []ResponseItem) []MemoryFact {
	text := strings.ToLower(extractInputText(inputItems))
	facts := make([]MemoryFact, 0, 3)

	if value, ok := pickSuffix(text, "my name is "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-name",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_name",
			Key:        "name",
			Value:      value,
			Confidence: 0.95,
		})
	}

	if value, ok := pickSuffix(text, "i like "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-like",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_like",
			Key:        "like",
			Value:      value,
			Confidence: 0.85,
		})
	}

	if value, ok := pickSuffix(text, "i prefer "); ok {
		facts = append(facts, MemoryFact{
			ID:         turnID + "-fact-prefer",
			TS:         ts,
			Type:       "fact_upsert",
			FactID:     "user_preference",
			Key:        "preference",
			Value:      value,
			Confidence: 0.85,
		})
	}

	return facts
}

// pickSuffix extracts short trailing value after marker until sentence separator.
func pickSuffix(text, marker string) (string, bool) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}

	sub := text[idx+len(marker):]
	for _, sep := range []string{"\n", ".", "!", "?", ",", ";"} {
		if pos := strings.Index(sub, sep); pos >= 0 {
			sub = sub[:pos]
		}
	}

	value := strings.TrimSpace(sub)
	if value == "" {
		return "", false
	}

	if len(value) > 64 {
		value = strings.TrimSpace(value[:64])
	}

	return value, true
}

// marshalJSONL marshals a slice/struct into JSONL text with trailing newline.
func marshalJSONL(v any) (string, error) {
	switch records := v.(type) {
	case []LogEvent:
		return marshalLogEvents(records)
	case []MemoryFact:
		return marshalFacts(records)
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			return "", errors.Wrap(err, "marshal generic")
		}
		if string(buf) == "null" {
			return "", nil
		}
		return string(buf) + "\n", nil
	}
}

// marshalLogEvents marshals log events into JSONL string.
func marshalLogEvents(events []LogEvent) (string, error) {
	if len(events) == 0 {
		return "", nil
	}

	lines := make([]string, 0, len(events))
	for _, event := range events {
		buf, err := json.Marshal(event)
		if err != nil {
			return "", errors.Wrap(err, "marshal event")
		}
		lines = append(lines, string(buf))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// marshalFacts marshals memory facts into JSONL string.
func marshalFacts(facts []MemoryFact) (string, error) {
	if len(facts) == 0 {
		return "", nil
	}

	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		buf, err := json.Marshal(fact)
		if err != nil {
			return "", errors.Wrap(err, "marshal fact")
		}
		lines = append(lines, string(buf))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// validateBeforeTurnInput validates required fields for BeforeTurn.
func validateBeforeTurnInput(in BeforeTurnInput) error {
	if strings.TrimSpace(in.Project) == "" {
		return errors.Errorf("project is required")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return errors.Errorf("session_id is required")
	}
	if strings.TrimSpace(in.TurnID) == "" {
		return errors.Errorf("turn_id is required")
	}
	if len(in.CurrentInput) == 0 {
		return errors.Errorf("current_input is required")
	}

	return nil
}

// validateAfterTurnInput validates required fields for AfterTurn.
func validateAfterTurnInput(in AfterTurnInput) error {
	if strings.TrimSpace(in.Project) == "" {
		return errors.Errorf("project is required")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return errors.Errorf("session_id is required")
	}
	if strings.TrimSpace(in.TurnID) == "" {
		return errors.Errorf("turn_id is required")
	}

	return nil
}

// sessionBasePath builds standardized memory directory path per session.
func sessionBasePath(sessionID string) string {
	return "/memory/" + sessionID
}

// contains checks whether value exists in list.
func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
}
