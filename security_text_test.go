package utils

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDedentPreservesZeroIndent checks that unindented content is never sliced away.
func TestDedentPreservesZeroIndent(t *testing.T) {
	for _, input := range []string{"    safe\nx", "    safe\nunindented", "x\n    safe", "    中文\n界"} {
		t.Run(input, func(t *testing.T) {
			require.NotPanics(t, func() { require.Equal(t, input, Dedent(input)) })
		})
	}
}

// TestURLMaskingDoesNotExposePassword verifies IPv6 and literal replacement masks.
func TestURLMaskingDoesNotExposePassword(t *testing.T) {
	for _, host := range []string{"example.invalid", "127.0.0.1:8443", "[::1]:8443"} {
		for _, mask := range []string{"*****", "$0", "${1}", `dollar$and\backslash`} {
			t.Run(host+"/"+mask, func(t *testing.T) {
				out := URLMasking("https://user:SYNTHETIC_SECRET@"+host+"/path?q=ok", mask)
				require.NotContains(t, out, "SYNTHETIC_SECRET")
				parsed, err := url.Parse(out)
				require.NoError(t, err)
				password, ok := parsed.User.Password()
				require.True(t, ok)
				require.Equal(t, mask, password)
				require.Equal(t, host, parsed.Host)
				require.Equal(t, "/path", parsed.Path)
				require.Equal(t, "q=ok", parsed.RawQuery)
			})
		}
	}
}

// TestJaegerParsingBounds verifies bounded diagnostics for tiny malformed fixtures.
func TestJaegerParsingBounds(t *testing.T) {
	for _, input := range []string{"1:1:0:" + strings.Repeat("ab", 32), strings.Repeat(":", 128), strings.Repeat("f", 65) + ":1:0:1"} {
		_, _, _, _, err := JaegerTracingID(input).Parse()
		require.Error(t, err)
		require.LessOrEqual(t, len(err.Error()), 96)
		require.NotContains(t, err.Error(), strings.Repeat("ab", 32))
		require.NotContains(t, err.Error(), strings.Repeat("f", 65))
	}
}

// TestFlattenMapDoesNotOverwriteCollision checks that legacy normalization cannot overwrite ambiguous data.
func TestFlattenMapDoesNotOverwriteCollision(t *testing.T) {
	input := map[string]any{"user.is_admin": false, "user": map[string]any{"is_admin": true}}
	FlattenMap(input, ".")
	require.Equal(t, map[string]any{"user.is_admin": false, "user": map[string]any{"is_admin": true}}, input)
}
