package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity75MnemonicDiagnostics ensures invalid text never appears in returned errors.
func TestSecurity75MnemonicDiagnostics(t *testing.T) {
	for _, input := range []string{"S3CRET!", strings.Repeat("S3CRET!", 3000), strings.Repeat(" ", 2<<20), strings.Repeat("abandon ", maxEncodedMnemonicWords+1)} {
		result, err := MnemonicToBytes(input)
		require.Error(t, err)
		require.Nil(t, result)
		require.LessOrEqual(t, len(err.Error()), 160)
		require.NotContains(t, err.Error(), "S3CRET!")
	}
	_, err := mnemonicWordsToBytes([]string{"S3CRET!"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "S3CRET!")
}

// BenchmarkSecurity75OversizedText measures rejection of a bounded 1.12 MB public-word fixture.
// The setup allocation is excluded; no live service or exhaustion workload is used.
func BenchmarkSecurity75OversizedText(b *testing.B) {
	input := strings.Repeat("abandon ", 140000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := MnemonicToBytes(input); err == nil {
			b.Fatal("oversized mnemonic accepted")
		}
	}
}
