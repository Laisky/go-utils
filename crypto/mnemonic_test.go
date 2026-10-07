package crypto

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// Standard BIP39
// -----------------------------------------------------------------------

func TestNewMnemonic(t *testing.T) {
	t.Parallel()

	for _, bits := range []int{128, 160, 192, 224, 256} {
		t.Run(strings.Repeat("_", bits/32), func(t *testing.T) {
			m, err := NewMnemonic(bits)
			require.NoError(t, err)

			words := strings.Fields(m)
			expectedWords := (bits + bits/32) / 11
			require.Len(t, words, expectedWords)
			require.True(t, ValidateMnemonic(m))
		})
	}
}

func TestNewMnemonic_InvalidBits(t *testing.T) {
	t.Parallel()

	for _, bits := range []int{0, 64, 100, 129, 512} {
		_, err := NewMnemonic(bits)
		require.Error(t, err)
	}
}

func TestEntropyToMnemonic_RoundTrip(t *testing.T) {
	t.Parallel()

	for _, size := range []int{16, 20, 24, 28, 32} {
		t.Run("", func(t *testing.T) {
			entropy := make([]byte, size)
			_, err := rand.Read(entropy)
			require.NoError(t, err)

			mnemonic, err := EntropyToMnemonic(entropy)
			require.NoError(t, err)
			require.True(t, ValidateMnemonic(mnemonic))

			recovered, err := MnemonicToEntropy(mnemonic)
			require.NoError(t, err)
			require.Equal(t, entropy, recovered)
		})
	}
}

func TestEntropyToMnemonic_InvalidSize(t *testing.T) {
	t.Parallel()

	_, err := EntropyToMnemonic([]byte{1, 2, 3})
	require.Error(t, err)
}

func TestMnemonicToEntropy_Invalid(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToEntropy("not a valid mnemonic at all")
	require.Error(t, err)
}

func TestMnemonicToSeed(t *testing.T) {
	t.Parallel()

	mnemonic, err := NewMnemonic(128)
	require.NoError(t, err)

	seed, err := MnemonicToSeed(mnemonic, "")
	require.NoError(t, err)
	require.Len(t, seed, 64) // 512 bits

	// Same mnemonic + passphrase → same seed
	seed2, err := MnemonicToSeed(mnemonic, "")
	require.NoError(t, err)
	require.Equal(t, seed, seed2)

	// Different passphrase → different seed
	seedWithPass, err := MnemonicToSeed(mnemonic, "my secret")
	require.NoError(t, err)
	require.NotEqual(t, seed, seedWithPass)
}

func TestMnemonicToSeed_InvalidMnemonic(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToSeed("invalid words here", "")
	require.Error(t, err)
}

func TestValidateMnemonic(t *testing.T) {
	t.Parallel()

	m, err := NewMnemonic(256)
	require.NoError(t, err)
	require.True(t, ValidateMnemonic(m))

	require.False(t, ValidateMnemonic(""))
	require.False(t, ValidateMnemonic("abandon abandon abandon"))
	require.False(t, ValidateMnemonic("notaword notaword notaword notaword notaword notaword notaword notaword notaword notaword notaword notaword"))
}

// Known test vector from BIP39 spec (English, no passphrase)
func TestBIP39_KnownVector(t *testing.T) {
	t.Parallel()

	// Vector: entropy 00000000000000000000000000000000
	entropy := make([]byte, 16)
	mnemonic, err := EntropyToMnemonic(entropy)
	require.NoError(t, err)
	require.Equal(t, "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", mnemonic)

	recovered, err := MnemonicToEntropy(mnemonic)
	require.NoError(t, err)
	require.Equal(t, entropy, recovered)
}

// -----------------------------------------------------------------------
// Bit manipulation helpers
// -----------------------------------------------------------------------

func Test_extract11Bits(t *testing.T) {
	t.Parallel()

	// 0xFF 0xFF = 1111111111111111 → first 11 bits = 11111111111 = 2047
	data := []byte{0xFF, 0xFF}
	require.Equal(t, uint16(2047), extract11Bits(data, 0))

	// 0x00 0x00 → first 11 bits = 0
	data = []byte{0x00, 0x00}
	require.Equal(t, uint16(0), extract11Bits(data, 0))

	// 0x80 0x00 = 10000000 00000000 → first 11 bits = 10000000000 = 1024
	data = []byte{0x80, 0x00}
	require.Equal(t, uint16(1024), extract11Bits(data, 0))
}

func Test_write11Bits(t *testing.T) {
	t.Parallel()

	data := make([]byte, 2)
	write11Bits(data, 0, 2047)
	require.Equal(t, uint16(2047), extract11Bits(data, 0))

	data = make([]byte, 2)
	write11Bits(data, 0, 1024)
	require.Equal(t, uint16(1024), extract11Bits(data, 0))
}

func Test_bitRoundTrip(t *testing.T) {
	t.Parallel()

	// Write and read multiple 11-bit values
	data := make([]byte, 6) // 48 bits → 4 full 11-bit values + 4 bits
	vals := []uint16{100, 2047, 0, 1234}
	for i, v := range vals {
		write11Bits(data, i*11, v)
	}
	for i, expected := range vals {
		require.Equal(t, expected, extract11Bits(data, i*11))
	}
}

// -----------------------------------------------------------------------
// bitsToMnemonicWords / mnemonicWordsToBytes round-trip
// -----------------------------------------------------------------------

func Test_wordsRoundTrip(t *testing.T) {
	t.Parallel()

	original := make([]byte, 32)
	_, err := rand.Read(original)
	require.NoError(t, err)

	words := bitsToMnemonicWords(original)
	recovered, err := mnemonicWordsToBytes(words)
	require.NoError(t, err)

	// recovered may be longer due to ceil division; compare only original length
	require.True(t, bytes.HasPrefix(recovered, original) || bytes.Equal(recovered[:len(original)], original))
}

// -----------------------------------------------------------------------
// Unused import guard (big.Int)
// -----------------------------------------------------------------------

var _ = new(big.Int)
