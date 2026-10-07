package crypto

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity75BoundedTokenizer checks canonical whitespace and exact token-count limits.
func TestSecurity75BoundedTokenizer(t *testing.T) {
	got, err := splitBoundedMnemonic(" \tabandon\nability\u2003zoo\r\n")
	require.NoError(t, err)
	require.Equal(t, []string{"abandon", "ability", "zoo"}, got)
	input := strings.Repeat("abandon ", maxEncodedMnemonicWords)
	got, err = splitBoundedMnemonic(input)
	require.NoError(t, err)
	require.Len(t, got, maxEncodedMnemonicWords)
	_, err = splitBoundedMnemonic(input + "abandon")
	require.Error(t, err)
	for _, invalid := range []string{"", " \n\t", "123456789", strings.Repeat(" ", maxMnemonicTextBytes+1)} {
		_, err := splitBoundedMnemonic(invalid)
		require.Error(t, err)
	}
	huge := strings.Repeat(" ", maxMnemonicTextBytes+1)
	allocations := testing.AllocsPerRun(100, func() { _, _ = splitBoundedMnemonic(huge) })
	require.LessOrEqual(t, allocations, float64(16))
}

// TestSecurity75WireCompatibility exercises the real BIP39 dictionary and the entire extended codec.
// This test requires the complete repository dependencies, not the invalid-input harness.
func TestSecurity75WireCompatibility(t *testing.T) {
	for _, size := range []int{1, 31, 1024, maxMnemonicDataLen} {
		input := bytes.Repeat([]byte{0x5a}, size)
		phrase, err := BytesToMnemonic(input)
		require.NoError(t, err)
		spaced := " \t" + strings.ReplaceAll(phrase, " ", "\n\t") + "\r\n"
		got, err := MnemonicToBytes(spaced)
		require.NoError(t, err)
		require.Equal(t, input, got)
	}
	_, err := MnemonicToPrikey("S3CRET!", WithMnemonicPassphrase("synthetic passphrase"))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "S3CRET!")
}
