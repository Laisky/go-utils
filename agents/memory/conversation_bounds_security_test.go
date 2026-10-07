package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// security69Items supplies explicit stable identities to keep boundary tests independent of text normalization.
func security69Items() []ResponseItem {
	return []ResponseItem{{Type: "message", Role: "user", Metadata: map[string]string{"item_id": "one"}},
		{Type: "message", Role: "user", Metadata: map[string]string{"item_id": "two"}}}
}

// TestSecurity69Resolver preserves legacy clamping while preventing addition overflow.
func TestSecurity69Resolver(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct{ start, count, wantStart, wantCount int }{
		{1, maxInt, 1, 1}, {2, maxInt, 2, 0}, {maxInt, maxInt, 2, 0},
		{-1, maxInt, 0, 2}, {1, 0, 1, 1}, {1, -1, 1, 1}, {0, 1, 0, 1}, {1, 1, 1, 1},
	} {
		_, start, count := resolveConversationItems(security69Items(), nil, tc.start, tc.count)
		require.Equal(t, tc.wantStart, start)
		require.Equal(t, tc.wantCount, count)
		require.NotPanics(t, func() {
			_, err := buildNormalizedConversation(security69Items(), start, count)
			require.NoError(t, err)
		})
	}
	items, start, count := resolveConversationItems(nil, security69Items(), maxInt, maxInt)
	require.Len(t, items, 2)
	require.Equal(t, 0, start)
	require.Equal(t, 2, count)
}

// TestSecurity69Builder rejects invalid slices before any slicing or allocation.
func TestSecurity69Builder(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, tc := range []struct{ start, count int }{{1, maxInt}, {2, maxInt}, {maxInt, 1}, {-1, 1}, {0, -1}, {minInt, 1}, {1, minInt}} {
		require.NotPanics(t, func() {
			got, err := buildNormalizedConversation(security69Items(), tc.start, tc.count)
			require.Error(t, err)
			require.Nil(t, got.AllItems)
		})
	}
	original := security69Items()
	got, err := buildNormalizedConversation(original, 1, 1)
	require.NoError(t, err)
	require.Len(t, got.HistoryItems, 1)
	require.Len(t, got.CurrentItems, 1)
	got.CurrentItems[0].Role = "changed"
	require.Equal(t, "user", original[1].Role)
}

// TestSecurity69BeforeAfter verifies both normalization entry points for oversized counts.
func TestSecurity69BeforeAfter(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	require.NotPanics(t, func() {
		before, err := normalizeBeforeTurnConversation(BeforeTurnInput{ConversationItems: security69Items(), CurrentInputStart: 1, CurrentInputCount: maxInt})
		require.NoError(t, err)
		require.Equal(t, 1, before.CurrentCount)
		after, err := normalizeAfterTurnConversation(AfterTurnInput{ConversationItems: security69Items(), CurrentInputStart: 1, CurrentInputCount: maxInt})
		require.NoError(t, err)
		require.Equal(t, 1, after.CurrentCount)
		_, err = normalizeBeforeTurnConversation(BeforeTurnInput{ConversationItems: security69Items(), CurrentInputStart: 2, CurrentInputCount: maxInt})
		require.Error(t, err)
		after, err = normalizeAfterTurnConversation(AfterTurnInput{ConversationItems: security69Items(), CurrentInputStart: 2, CurrentInputCount: maxInt})
		require.NoError(t, err)
		require.Equal(t, 0, after.CurrentCount)
	})
}

// FuzzSecurity69Bounds checks all integer boundaries against a subtraction-based oracle.
func FuzzSecurity69Bounds(f *testing.F) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range [][2]int{{1, maxInt}, {2, maxInt}, {0, 0}, {-1, 1}, {1, 1}} {
		f.Add(tc[0], tc[1])
	}
	f.Fuzz(func(t *testing.T, start, count int) {
		items := security69Items()
		valid := start >= 0 && count >= 0 && start <= len(items) && count <= len(items)-start
		got, err := buildNormalizedConversation(items, start, count)
		if valid {
			require.NoError(t, err)
			require.Len(t, got.CurrentItems, count)
		} else {
			require.Error(t, err)
		}
		normalized, s, c := resolveConversationItems(items, nil, start, count)
		_, err = buildNormalizedConversation(normalized, s, c)
		require.NoError(t, err)
	})
}
