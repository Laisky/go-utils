package utils

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJaegerParseFlagByteRange verifies that the one-byte flag component maps
// every accepted hexadecimal value to the identical byte and rejects anything
// that would not fit, so the uint64-to-byte conversion in Parse can never
// truncate. It guards the gosec G115 lint cleanup of JaegerTracingID.Parse.
func TestJaegerParseFlagByteRange(t *testing.T) {
	t.Parallel()
	for value := range 256 {
		for _, encoded := range []string{strconv.FormatUint(uint64(value), 16), formatTwoDigitHex(value)} {
			_, _, _, flag, err := JaegerTracingID("1:2:3:" + encoded).Parse()
			require.NoError(t, err, encoded)
			require.Equal(t, byte(value), flag, encoded)
		}
	}
	for _, encoded := range []string{"100", "1ff", "fff", "0ff", "-1", "+1", "0x1", "g", "", "1 "} {
		_, _, _, flag, err := JaegerTracingID("1:2:3:" + encoded).Parse()
		require.Error(t, err, encoded)
		require.Zero(t, flag, encoded)
	}
}

// TestJaegerParseComponentSyntax verifies the shared hexadecimal validation of
// all four components: mixed-case digits are accepted, while signs, prefixes,
// whitespace and non-hexadecimal letters are rejected in every position.
func TestJaegerParseComponentSyntax(t *testing.T) {
	t.Parallel()
	trace, span, parent, flag, err := JaegerTracingID("aBcDeF0123456789:F:a:Bc").Parse()
	require.NoError(t, err)
	require.Equal(t, uint64(0xabcdef0123456789), trace)
	require.Equal(t, uint64(0xf), span)
	require.Equal(t, uint64(0xa), parent)
	require.Equal(t, byte(0xbc), flag)
	valid := []string{"1", "2", "3", "4"}
	for position := range valid {
		for _, bad := range []string{"g", "+1", "-1", "0x1", " 1", "1\n", "١"} {
			fields := append([]string(nil), valid...)
			fields[position] = bad
			raw := fields[0] + ":" + fields[1] + ":" + fields[2] + ":" + fields[3]
			_, _, _, _, err := JaegerTracingID(raw).Parse()
			require.Error(t, err, raw)
		}
	}
}

// formatTwoDigitHex returns value as exactly two lowercase hexadecimal digits.
// It takes a value in [0, 255] and returns its zero-padded encoding.
func formatTwoDigitHex(value int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[value>>4], digits[value&0xf]})
}
