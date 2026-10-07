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

// TestBytesToMnemonic_RoundTrip verifies that random byte slices of 13 sizes between 1 and 1024
// bytes, encoded by BytesToMnemonic, decode back to the identical bytes via MnemonicToBytes.
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

// TestBytesToMnemonic_Deterministic verifies that encoding the same input twice with
// BytesToMnemonic produces the identical mnemonic, since the extended format has no random parts.
func TestBytesToMnemonic_Deterministic(t *testing.T) {
	t.Parallel()

	data := []byte("deterministic test input")
	m1, err := BytesToMnemonic(data)
	require.NoError(t, err)

	m2, err := BytesToMnemonic(data)
	require.NoError(t, err)

	require.Equal(t, m1, m2)
}

// TestBytesToMnemonic_DifferentInputs verifies that two 3-byte inputs differing only in their last
// byte are encoded by BytesToMnemonic into different mnemonics.
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

// TestBytesToMnemonic_EmptyInput verifies that BytesToMnemonic rejects both a nil slice (with a
// "must not be empty" error) and a non-nil empty slice.
func TestBytesToMnemonic_EmptyInput(t *testing.T) {
	t.Parallel()

	_, err := BytesToMnemonic(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")

	_, err = BytesToMnemonic([]byte{})
	require.Error(t, err)
}

// TestBytesToMnemonic_MaxSize verifies that BytesToMnemonic rejects an input one byte longer than
// maxMnemonicDataLen with a "too large" error.
func TestBytesToMnemonic_MaxSize(t *testing.T) {
	t.Parallel()

	// Just over the limit
	oversize := make([]byte, maxMnemonicDataLen+1)
	_, err := BytesToMnemonic(oversize)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too large")
}

// TestMnemonicToBytes_Empty verifies that MnemonicToBytes returns an error for an empty mnemonic.
func TestMnemonicToBytes_Empty(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToBytes("")
	require.Error(t, err)
}

// TestMnemonicToBytes_InvalidWord verifies that MnemonicToBytes rejects a phrase made of words that
// are absent from the BIP39 English list with a "not found in BIP39 word list" error.
func TestMnemonicToBytes_InvalidWord(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToBytes("xyzzy foobar baz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in BIP39 word list")
}

// TestMnemonicToBytes_TooManyWords verifies that MnemonicToBytes rejects a phrase containing
// maxEncodedMnemonicWords+1 valid words with a "mnemonic too long" error raised while the phrase is
// being tokenized, before any word is decoded.
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

// TestMnemonicToBytes_CorruptChecksum verifies that replacing the final word of a mnemonic produced
// by BytesToMnemonic with a different BIP39 word makes MnemonicToBytes fail with a checksum error.
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

// TestMnemonicToBytes_UnsupportedVersion verifies that a mnemonic whose decoded version byte is
// rewritten to 0xFF and re-encoded into words is rejected by MnemonicToBytes with an "unsupported
// mnemonic version" error, because the version check runs before checksum verification.
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

// TestBytesToMnemonic_SingleByte verifies that every possible one-byte input, 0x00 through 0xFF,
// round-trips unchanged through BytesToMnemonic and MnemonicToBytes.
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

// TestBytesToMnemonic_AllZeros verifies that a 32-byte all-zero input round-trips unchanged through
// BytesToMnemonic and MnemonicToBytes.
func TestBytesToMnemonic_AllZeros(t *testing.T) {
	t.Parallel()

	data := make([]byte, 32)
	m, err := BytesToMnemonic(data)
	require.NoError(t, err)

	recovered, err := MnemonicToBytes(m)
	require.NoError(t, err)
	require.Equal(t, data, recovered)
}

// TestBytesToMnemonic_AllOnes verifies that a 32-byte input of 0xFF bytes round-trips unchanged
// through BytesToMnemonic and MnemonicToBytes.
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

// TestBytesToMnemonic_LargeData verifies that a 4096-byte random input round-trips unchanged through
// BytesToMnemonic and MnemonicToBytes.
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

// BenchmarkBytesToMnemonic_32B measures BytesToMnemonic encoding a fixed 32-byte random input.
func BenchmarkBytesToMnemonic_32B(b *testing.B) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	b.ResetTimer()
	for range b.N {
		_, _ = BytesToMnemonic(data)
	}
}

// BenchmarkMnemonicToBytes_32B measures MnemonicToBytes decoding the mnemonic of a fixed 32-byte
// random input, excluding the one-time encoding done before the timer is reset.
func BenchmarkMnemonicToBytes_32B(b *testing.B) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	m, _ := BytesToMnemonic(data)
	b.ResetTimer()
	for range b.N {
		_, _ = MnemonicToBytes(m)
	}
}
