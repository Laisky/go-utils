package utils

import (
	"math"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFlattenMapSafeRejectsAmbiguity verifies collision rejection is transactional and value-independent.
func TestFlattenMapSafeRejectsAmbiguity(t *testing.T) {
	for _, values := range [][2]any{{false, true}, {"same", "same"}, {nil, nil}, {map[string]any{}, map[string]any{}}} {
		for _, delimiter := range []string{".", "/", "::"} {
			input := map[string]any{"a" + delimiter + "b": values[0], "a": map[string]any{"b": values[1]}}
			result, err := FlattenMapSafe(input, delimiter)
			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, input, 2)
			require.Equal(t, values[0], input["a"+delimiter+"b"])
			require.Equal(t, map[string]any{"b": values[1]}, input["a"])
		}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	_, err := FlattenMapSafe(cycle, ".")
	require.Error(t, err)
	require.Len(t, cycle, 1)
	_, err = FlattenMapSafe(map[string]any{"a": 1}, "")
	require.Error(t, err)
}

// TestFlattenMapSafeValidInput preserves empty maps, scalar values, and input structure.
func TestFlattenMapSafeValidInput(t *testing.T) {
	input := map[string]any{"a": map[string]any{"b": true}, "empty": map[string]any{}, "nil": nil, "": 1}
	flat, err := FlattenMapSafe(input, ".")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"a.b": true, "empty": map[string]any{}, "nil": nil, "": 1}, flat)
	require.Equal(t, map[string]any{"b": true}, input["a"])
	flat["a.b"] = false
	require.Equal(t, map[string]any{"b": true}, input["a"])
	var nilMap map[string]any
	flat, err = FlattenMapSafe(nilMap, ".")
	require.NoError(t, err)
	require.Nil(t, flat)
	FlattenMap(nilMap, ".")
}

// TestDedentWhitespaceBoundaries covers blank trimming, mixed margins, Unicode, and option bounds.
func TestDedentWhitespaceBoundaries(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""}, {"\n \t\n", ""}, {"\n    first\n\n    next\n\n", "first\n\nnext"},
		{"\tfirst\n    next", "first\nnext"}, {"    中文\n  文本", "  中文\n文本"},
		{"\tfirst\nx", "    first\nx"},
	} {
		require.Equal(t, tc.want, Dedent(tc.in))
	}
	for _, width := range []int{-1, math.MinInt, 257, math.MaxInt} {
		require.Equal(t, "    first\nx", Dedent("\tfirst\nx", WithReplaceTabBySpaces(width)))
	}
	require.Equal(t, "first\nx", Dedent("\tfirst\nx", WithReplaceTabBySpaces(0)))
}

// TestJaegerParsingBoundaries exercises accepted legacy values and exact component limits.
func TestJaegerParsingBoundaries(t *testing.T) {
	trace, span, parent, flag, err := JaegerTracingID("ffffffffffffffff:FFFFFFFFFFFFFFFF:ffffffffffffffff:ff").Parse()
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint64), trace)
	require.Equal(t, trace, span)
	require.Equal(t, trace, parent)
	require.Equal(t, byte(255), flag)
	_, _, parent, flag, err = JaegerTracingID("1:2::4").Parse()
	require.NoError(t, err)
	require.Zero(t, parent)
	require.Equal(t, byte(4), flag)
	for _, raw := range []string{"", "1:2:3", "1:2:3:4:5", ":2:3:4", "1::3:4", "1:2:3:", "1:2:3:100", "1:2:3:gg", "-1:2:3:4", "1:2:3: 4"} {
		_, _, _, _, err := JaegerTracingID(raw).Parse()
		require.Error(t, err, raw)
		require.LessOrEqual(t, len(err.Error()), 96)
	}
}

// TestMaskURLPasswordMalformedAndEscaped handles malformed URLs without echoing credentials.
func TestMaskURLPasswordMalformedAndEscaped(t *testing.T) {
	for _, raw := range []string{"https://u:SENTINEL_SECRET@host/%zz", "https://u:SENTINEL_SECRET@[::1", "https://u:SENTINEL_SECRET@host/\n", "u:SENTINEL_SECRET@host"} {
		got, err := MaskURLPassword(raw, "*****")
		require.Error(t, err)
		require.Empty(t, got)
		require.NotContains(t, err.Error(), "SENTINEL_SECRET")
		require.Equal(t, "[invalid URL]", URLMasking(raw, "*****"))
	}
	for _, raw := range []string{"https://us%40er:p%40ss%3Aword@[::1]:443/p?q=x#f", "//u:secret@example.invalid/p"} {
		got, err := MaskURLPassword(raw, ":/@*%$\\")
		require.NoError(t, err)
		before, err := url.Parse(raw)
		require.NoError(t, err)
		after, err := url.Parse(got)
		require.NoError(t, err)
		password, exists := after.User.Password()
		require.True(t, exists)
		require.Equal(t, ":/@*%$\\", password)
		require.Equal(t, before.User.Username(), after.User.Username())
		require.Equal(t, before.Host, after.Host)
		require.Equal(t, before.Path, after.Path)
		require.Equal(t, before.RawQuery, after.RawQuery)
		require.Equal(t, before.Fragment, after.Fragment)
	}
	for _, raw := range []string{"https://example.invalid/path", "https://user@example.invalid/path"} {
		got, err := MaskURLPassword(raw, "*****")
		require.NoError(t, err)
		require.Equal(t, raw, got)
	}
}

// FuzzTextBoundaryHelpers verifies bounded parsing errors and non-panicking indentation handling.
func FuzzTextBoundaryHelpers(f *testing.F) {
	for _, raw := range []string{"    safe\nx", "1:2::4", strings.Repeat(":", 128), "\t中文\n界"} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 16384 {
			t.Skip()
		}
		_ = Dedent(raw)
		_, _, _, _, err := JaegerTracingID(raw).Parse()
		if err != nil {
			require.LessOrEqual(t, len(err.Error()), 96)
		}
	})
}
