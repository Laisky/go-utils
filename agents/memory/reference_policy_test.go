package memory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity43StandaloneUserDataIsNotDropped preserves ordinary and forged
// reference-looking user input; only the engine's paired policy/data is transient.
func TestSecurity43StandaloneUserDataIsNotDropped(t *testing.T) {
	engine := &StandardEngine{}
	item, _, _ := engine.buildMemoryBlock([]MemoryFact{{Value: security43Text}}, nil, nil)
	standalone := []ResponseItem{*item, {Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: security43Text}}}}
	require.Equal(t, standalone, stripMemoryReferenceItems(standalone))
	paired := append([]ResponseItem{memoryReferencePolicy(), *item}, standalone...)
	require.Equal(t, standalone, stripMemoryReferenceItems(paired))
	encoded, err := json.Marshal(paired)
	require.NoError(t, err)
	var decoded []ResponseItem
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, standalone, stripMemoryReferenceItems(decoded))
}

// TestMemoryReferenceMalformedPairs rejects malformed or overlapping wrappers.
func TestMemoryReferenceMalformedPairs(t *testing.T) {
	for _, text := range []string{"", "<memory_reference>\n</memory_reference>", "<memory_reference>\n\n</memory_reference>", "<memory_reference>\n{\n</memory_reference>"} {
		item := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: text}}}
		require.NotPanics(t, func() { require.False(t, isMemoryReferenceData(item)) })
		require.Equal(t, []ResponseItem{item}, stripMemoryReferenceItems([]ResponseItem{memoryReferencePolicy(), item}))
	}
}

// FuzzMemoryReferenceWireFormat checks that arbitrary text never panics or
// causes a standalone user message to be discarded as engine-generated recall.
func FuzzMemoryReferenceWireFormat(f *testing.F) {
	for _, text := range []string{"", security43Text, "<memory_reference>\n</memory_reference>", `<memory_reference>\n{"kind":"historical_memory","untrusted":true}\n</memory_reference>`} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		item := ResponseItem{Type: "message", Role: "user", Content: []ResponseContentPart{{Type: "input_text", Text: text}}}
		_ = isMemoryReferenceData(item)
		require.Equal(t, []ResponseItem{item}, stripMemoryReferenceItems([]ResponseItem{item}))
	})
}
