package crypto

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// Extended BytesToMnemonic / MnemonicToBytes
// -----------------------------------------------------------------------

func TestBytesToMnemonic_RoundTrip(t *testing.T) {
	t.Parallel()

	sizes := []int{1, 2, 10, 31, 32, 33, 48, 64, 100, 255, 256, 512, 1024}
	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			data := make([]byte, size)
			_, err := rand.Read(data)
			require.NoError(t, err)

			mnemonic, err := BytesToMnemonic(data)
			require.NoError(t, err)

			recovered, err := MnemonicToBytes(mnemonic)
			require.NoError(t, err)
			require.Equal(t, data, recovered)
		})
	}
}

func TestBytesToMnemonic_Deterministic(t *testing.T) {
	t.Parallel()

	data := []byte("deterministic test input")
	m1, err := BytesToMnemonic(data)
	require.NoError(t, err)

	m2, err := BytesToMnemonic(data)
	require.NoError(t, err)

	require.Equal(t, m1, m2)
}

func TestBytesToMnemonic_DifferentInputs(t *testing.T) {
	t.Parallel()

	d1 := []byte{0x00, 0x01, 0x02}
	d2 := []byte{0x00, 0x01, 0x03}

	m1, err := BytesToMnemonic(d1)
	require.NoError(t, err)
	m2, err := BytesToMnemonic(d2)
	require.NoError(t, err)

	require.NotEqual(t, m1, m2)
}

func TestBytesToMnemonic_EmptyInput(t *testing.T) {
	t.Parallel()

	_, err := BytesToMnemonic(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")

	_, err = BytesToMnemonic([]byte{})
	require.Error(t, err)
}

func TestBytesToMnemonic_MaxSize(t *testing.T) {
	t.Parallel()

	// Just over the limit
	oversize := make([]byte, maxMnemonicDataLen+1)
	_, err := BytesToMnemonic(oversize)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too large")
}

func TestMnemonicToBytes_Empty(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToBytes("")
	require.Error(t, err)
}

func TestMnemonicToBytes_InvalidWord(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToBytes("xyzzy foobar baz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in BIP39 word list")
}

func TestMnemonicToBytes_TooManyWords(t *testing.T) {
	t.Parallel()

	words := make([]string, maxEncodedMnemonicWords+1)
	for i := range words {
		words[i] = mnemonicWordList[0]
	}

	_, err := MnemonicToBytes(strings.Join(words, " "))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mnemonic too long")
}

func TestMnemonicToBytes_CorruptChecksum(t *testing.T) {
	t.Parallel()

	data := []byte("checksum test data")
	mnemonic, err := BytesToMnemonic(data)
	require.NoError(t, err)

	// Replace the last word to corrupt checksum
	words := strings.Fields(mnemonic)
	lastWord := words[len(words)-1]
	// Pick a different word
	for _, w := range mnemonicWordList {
		if w != lastWord {
			words[len(words)-1] = w
			break
		}
	}

	_, err = MnemonicToBytes(strings.Join(words, " "))
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")
}

func TestMnemonicToBytes_UnsupportedVersion(t *testing.T) {
	t.Parallel()

	// Encode some data, then tamper with the version byte
	data := []byte("version test")
	mnemonic, err := BytesToMnemonic(data)
	require.NoError(t, err)

	// Decode to bytes, change version, re-encode
	words := strings.Fields(mnemonic)
	raw, err := mnemonicWordsToBytes(words)
	require.NoError(t, err)

	// Set version to 0xFF
	raw[0] = 0xFF
	// Re-encode (won't have valid checksum, but version check comes first)
	wordsNew := bitsToMnemonicWords(raw)
	_, err = MnemonicToBytes(strings.Join(wordsNew, " "))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported mnemonic version")
}

func TestBytesToMnemonic_SingleByte(t *testing.T) {
	t.Parallel()

	for b := 0; b < 256; b++ {
		data := []byte{byte(b)}
		m, err := BytesToMnemonic(data)
		require.NoError(t, err)

		recovered, err := MnemonicToBytes(m)
		require.NoError(t, err)
		require.Equal(t, data, recovered)
	}
}

func TestBytesToMnemonic_AllZeros(t *testing.T) {
	t.Parallel()

	data := make([]byte, 32)
	m, err := BytesToMnemonic(data)
	require.NoError(t, err)

	recovered, err := MnemonicToBytes(m)
	require.NoError(t, err)
	require.Equal(t, data, recovered)
}

func TestBytesToMnemonic_AllOnes(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte{0xFF}, 32)
	m, err := BytesToMnemonic(data)
	require.NoError(t, err)

	recovered, err := MnemonicToBytes(m)
	require.NoError(t, err)
	require.Equal(t, data, recovered)
}

// -----------------------------------------------------------------------
// Large data test for BytesToMnemonic
// -----------------------------------------------------------------------

func TestBytesToMnemonic_LargeData(t *testing.T) {
	t.Parallel()

	data := make([]byte, 4096)
	_, err := rand.Read(data)
	require.NoError(t, err)

	mnemonic, err := BytesToMnemonic(data)
	require.NoError(t, err)

	recovered, err := MnemonicToBytes(mnemonic)
	require.NoError(t, err)
	require.Equal(t, data, recovered)
}

// -----------------------------------------------------------------------
// Benchmark
// -----------------------------------------------------------------------

func BenchmarkBytesToMnemonic_32B(b *testing.B) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	b.ResetTimer()
	for range b.N {
		_, _ = BytesToMnemonic(data)
	}
}

func BenchmarkMnemonicToBytes_32B(b *testing.B) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	m, _ := BytesToMnemonic(data)
	b.ResetTimer()
	for range b.N {
		_, _ = MnemonicToBytes(m)
	}
}
