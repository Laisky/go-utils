package memory

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
)

// normalizedConversation stores caller history and current-turn slices after contract normalization.
type normalizedConversation struct {
	AllItems     []ResponseItem
	HistoryItems []ResponseItem
	CurrentItems []ResponseItem
	HistoryIDs   map[string]struct{}
	CurrentIDs   map[string]struct{}
	CurrentStart int
	CurrentCount int
}

// normalizeBeforeTurnConversation resolves legacy and V2 conversation input fields into one normalized shape.
func normalizeBeforeTurnConversation(in BeforeTurnInput) (normalizedConversation, error) {
	items, start, count := resolveConversationItems(in.ConversationItems, in.CurrentInput, in.CurrentInputStart, in.CurrentInputCount)
	if count == 0 {
		return normalizedConversation{}, newValidationError(ValidationErrorCodeCurrentInputRequired, "current_input", "current input items are required")
	}

	return buildNormalizedConversation(items, start, count)
}

// normalizeAfterTurnConversation resolves legacy and V2 after-turn fields into one normalized shape.
func normalizeAfterTurnConversation(in AfterTurnInput) (normalizedConversation, error) {
	items, start, count := resolveConversationItems(in.ConversationItems, in.InputItems, in.CurrentInputStart, in.CurrentInputCount)
	if len(items) == 0 {
		return buildNormalizedConversation(nil, 0, 0)
	}

	return buildNormalizedConversation(items, start, count)
}

// resolveConversationItems picks the effective conversation slice and current-turn boundary.
func resolveConversationItems(conversationItems, fallbackCurrent []ResponseItem, start, count int) ([]ResponseItem, int, int) {
	if len(conversationItems) == 0 {
		items := cloneResponseItems(fallbackCurrent)
		return items, 0, len(items)
	}

	items := cloneResponseItems(conversationItems)
	if start < 0 {
		start = 0
	}
	if start > len(items) {
		start = len(items)
	}
	if count <= 0 || start+count > len(items) {
		count = len(items) - start
	}

	return items, start, count
}

// buildNormalizedConversation builds one normalized conversation with stable identity sets.
func buildNormalizedConversation(items []ResponseItem, start, count int) (normalizedConversation, error) {
	if start < 0 || count < 0 || start > len(items) || start+count > len(items) {
		return normalizedConversation{}, errors.Errorf("invalid conversation boundary")
	}

	conversation := normalizedConversation{
		AllItems:     cloneResponseItems(items),
		HistoryItems: cloneResponseItems(items[:start]),
		CurrentItems: cloneResponseItems(items[start : start+count]),
		HistoryIDs:   make(map[string]struct{}, start),
		CurrentIDs:   make(map[string]struct{}, count),
		CurrentStart: start,
		CurrentCount: count,
	}

	for _, item := range conversation.HistoryItems {
		conversation.HistoryIDs[responseItemIdentity(item)] = struct{}{}
	}
	for _, item := range conversation.CurrentItems {
		conversation.CurrentIDs[responseItemIdentity(item)] = struct{}{}
	}

	return conversation, nil
}

// cloneResponseItems copies the slice header and item contents shallowly for safe manipulation.
func cloneResponseItems(items []ResponseItem) []ResponseItem {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]ResponseItem, len(items))
	copy(cloned, items)
	return cloned
}

// responseItemIdentity returns a stable identity for one response item.
func responseItemIdentity(item ResponseItem) string {
	if item.Metadata != nil {
		if itemID := strings.TrimSpace(item.Metadata["item_id"]); itemID != "" {
			return itemID
		}
	}

	normalized := map[string]any{
		"type":    strings.TrimSpace(item.Type),
		"role":    strings.TrimSpace(item.Role),
		"call_id": strings.TrimSpace(item.CallID),
		"output":  normalizeFactValue(item.Output),
		"content": normalizeResponseContentParts(item.Content),
	}
	buf, err := json.Marshal(normalized)
	if err != nil {
		return strings.TrimSpace(item.Type + ":" + item.Role + ":" + extractInputText([]ResponseItem{item}))
	}

	sum := sha1.Sum(buf)
	return fmt.Sprintf("item-%x", sum[:8])
}

// normalizeResponseContentParts converts content parts into a stable canonical representation.
func normalizeResponseContentParts(parts []ResponseContentPart) []map[string]string {
	if len(parts) == 0 {
		return nil
	}

	normalized := make([]map[string]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, map[string]string{
			"type":      strings.TrimSpace(part.Type),
			"text":      normalizeFactValue(part.Text),
			"image_url": strings.TrimSpace(part.ImageURL),
			"file_id":   strings.TrimSpace(part.FileID),
			"filename":  strings.TrimSpace(part.Filename),
		})
	}

	return normalized
}

// filterItemsByIdentity keeps items whose identities are not present in excluded.
func filterItemsByIdentity(items []ResponseItem, excluded map[string]struct{}) ([]ResponseItem, int) {
	if len(items) == 0 || len(excluded) == 0 {
		return cloneResponseItems(items), 0
	}

	filtered := make([]ResponseItem, 0, len(items))
	dropped := 0
	for _, item := range items {
		identity := responseItemIdentity(item)
		if _, ok := excluded[identity]; ok {
			dropped++
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered, dropped
}

// mergeIdentitySets creates one identity set from one or more source sets.
func mergeIdentitySets(sets ...map[string]struct{}) map[string]struct{} {
	total := 0
	for _, set := range sets {
		total += len(set)
	}
	merged := make(map[string]struct{}, total)
	for _, set := range sets {
		for key := range set {
			merged[key] = struct{}{}
		}
	}

	return merged
}
