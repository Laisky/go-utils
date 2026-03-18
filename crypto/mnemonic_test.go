package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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
// PrikeyToMnemonic / MnemonicToPrikey
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_Ed25519(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)
	require.NotEmpty(t, mnemonic)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEd, ok := recovered.(ed25519.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEd))
}

func TestPrikeyToMnemonic_ECDSA_P256(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEC, ok := recovered.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEC))
}

func TestPrikeyToMnemonic_ECDSA_P384(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEC, ok := recovered.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEC))
}

func TestPrikeyToMnemonic_ECDSA_P521(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEC, ok := recovered.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEC))
}

func TestPrikeyToMnemonic_RSA2048(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredRSA, ok := recovered.(*rsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredRSA))
}

func TestPrikeyToMnemonic_RSA4096(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredRSA, ok := recovered.(*rsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredRSA))
}

// -----------------------------------------------------------------------
// Passphrase-encrypted PrikeyToMnemonic
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_WithPassphrase_Ed25519(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	passphrase := "super-secret-passphrase-2026!"

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	// Correct passphrase recovers the key
	recovered, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	recoveredEd, ok := recovered.(ed25519.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEd))
}

func TestPrikeyToMnemonic_WithPassphrase_RSA(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	passphrase := "another-strong-pass!"

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	recoveredRSA, ok := recovered.(*rsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredRSA))
}

func TestPrikeyToMnemonic_WithPassphrase_ECDSA(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	passphrase := "ecdsa-pass!"

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic, WithMnemonicPassphrase(passphrase))
	require.NoError(t, err)

	recoveredEC, ok := recovered.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.True(t, prikey.Equal(recoveredEC))
}

func TestPrikeyToMnemonic_WrongPassphrase(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("correct"))
	require.NoError(t, err)

	_, err = MnemonicToPrikey(mnemonic, WithMnemonicPassphrase("wrong"))
	require.Error(t, err)
}

func TestPrikeyToMnemonic_NoPassphraseOnEncrypted(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("secret"))
	require.NoError(t, err)

	// Decoding without passphrase should fail
	_, err = MnemonicToPrikey(mnemonic)
	require.Error(t, err)
	require.Contains(t, err.Error(), "encrypted but no passphrase")
}

func TestWithMnemonicPassphrase_Empty(t *testing.T) {
	t.Parallel()

	var o mnemonicOption
	err := o.apply(WithMnemonicPassphrase(""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")
}

// -----------------------------------------------------------------------
// Encrypted format: different passphrases produce different mnemonics
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_DifferentPassphrases(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	m1, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("pass1"))
	require.NoError(t, err)

	m2, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("pass2"))
	require.NoError(t, err)

	// Different passphrases → different ciphertexts → different mnemonics
	// (due to random salt and IV)
	require.NotEqual(t, m1, m2)
}

// -----------------------------------------------------------------------
// Encrypted vs unencrypted: same key produces different mnemonics
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_EncryptedVsUnencrypted(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	plain, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	encrypted, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("pass"))
	require.NoError(t, err)

	require.NotEqual(t, plain, encrypted)

	// Both should recover the same key
	key1, err := MnemonicToPrikey(plain)
	require.NoError(t, err)
	key2, err := MnemonicToPrikey(encrypted, WithMnemonicPassphrase("pass"))
	require.NoError(t, err)

	require.True(t, key1.(ed25519.PrivateKey).Equal(key2))
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
// Signing round-trip: key recovered from mnemonic can sign/verify
// -----------------------------------------------------------------------

func TestMnemonicKey_CanSign_Ed25519(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recoveredKey, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	msg := []byte("test message for signing")
	sig, err := recoveredKey.(ed25519.PrivateKey).Sign(rand.Reader, msg, &ed25519.Options{})
	require.NoError(t, err)

	pubkey := prikey.Public().(ed25519.PublicKey)
	require.True(t, ed25519.Verify(pubkey, msg, sig))
}

func TestMnemonicKey_CanSign_RSA(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recoveredKey, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredRSA := recoveredKey.(*rsa.PrivateKey)
	msg := []byte("rsa signing test")
	sig, err := SignByRSAPKCS1v15WithSHA256(recoveredRSA, msg)
	require.NoError(t, err)
	require.NoError(t, VerifyByRSAPKCS1v15WithSHA256(&prikey.PublicKey, msg, sig))
}

func TestMnemonicKey_CanSign_ECDSA(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recoveredKey, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEC := recoveredKey.(*ecdsa.PrivateKey)
	msg := []byte("ecdsa signing test")
	r, s, err := SignByECDSAWithSHA256(recoveredEC, msg)
	require.NoError(t, err)
	require.True(t, VerifyByECDSAWithSHA256(&prikey.PublicKey, msg, r, s))
}

// -----------------------------------------------------------------------
// Edge cases for MnemonicToPrikey
// -----------------------------------------------------------------------

func TestMnemonicToPrikey_InvalidMnemonic(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToPrikey("abandon abandon abandon")
	require.Error(t, err)
}

func TestMnemonicToPrikey_EmptyMnemonic(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToPrikey("")
	require.Error(t, err)
}

// -----------------------------------------------------------------------
// Stability test: encoding is deterministic (no passphrase)
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_Deterministic(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	m1, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	m2, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	// Without passphrase, same key → same DER → same mnemonic
	require.Equal(t, m1, m2)
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
// Verify the encrypted marker byte detection
// -----------------------------------------------------------------------

func TestEncryptedMarkerDetection(t *testing.T) {
	t.Parallel()

	// Data that happens to start with the encrypted marker
	// should NOT be treated as encrypted by PrikeyToMnemonic/MnemonicToPrikey
	// because it goes through BytesToMnemonic first, then the inner data
	// is the raw DER which won't start with 0xE1

	// Manually test: if we encode data starting with 0xE1,
	// MnemonicToBytes should recover it correctly
	data := []byte{mnemonicEncryptedMarker, 0x01, 0x02, 0x03}
	m, err := BytesToMnemonic(data)
	require.NoError(t, err)

	recovered, err := MnemonicToBytes(m)
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

func BenchmarkPrikeyToMnemonic_Ed25519(b *testing.B) {
	_, prikey, _ := ed25519.GenerateKey(rand.Reader)
	b.ResetTimer()
	for range b.N {
		_, _ = PrikeyToMnemonic(prikey)
	}
}

// -----------------------------------------------------------------------
// Test ECDSA D-value preservation (scalar correctness)
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_ECDSA_ScalarPreserved(t *testing.T) {
	t.Parallel()

	prikey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEC := recovered.(*ecdsa.PrivateKey)

	// Verify the scalar D is identical
	require.Equal(t, 0, prikey.D.Cmp(recoveredEC.D))

	// Verify the public point is identical
	require.Equal(t, 0, prikey.PublicKey.X.Cmp(recoveredEC.PublicKey.X))
	require.Equal(t, 0, prikey.PublicKey.Y.Cmp(recoveredEC.PublicKey.Y))
}

// -----------------------------------------------------------------------
// Test RSA key component preservation
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_RSA_ComponentsPreserved(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredRSA := recovered.(*rsa.PrivateKey)

	require.Equal(t, 0, prikey.N.Cmp(recoveredRSA.N))
	require.Equal(t, prikey.E, recoveredRSA.E)
	require.Equal(t, 0, prikey.D.Cmp(recoveredRSA.D))
	require.Len(t, recoveredRSA.Primes, len(prikey.Primes))
	for i := range prikey.Primes {
		require.Equal(t, 0, prikey.Primes[i].Cmp(recoveredRSA.Primes[i]))
	}
}

// -----------------------------------------------------------------------
// Test Ed25519 seed preservation
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_Ed25519_SeedPreserved(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredEd := recovered.(ed25519.PrivateKey)

	// Seed bytes must be identical
	require.Equal(t, prikey.Seed(), recoveredEd.Seed())
	require.Equal(t, prikey.Public(), recoveredEd.Public())
}

// -----------------------------------------------------------------------
// Cross-validate: key from mnemonic works with existing sign functions
// -----------------------------------------------------------------------

func TestMnemonicKey_CrossValidate_SignByEd25519WithSHA512(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	msg := []byte("cross-validation test message")
	sig, err := SignByEd25519WithSHA512(recovered.(ed25519.PrivateKey), bytes.NewReader(msg))
	require.NoError(t, err)

	err = VerifyByEd25519WithSHA512(prikey.Public().(ed25519.PublicKey), bytes.NewReader(msg), sig)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------
// Large key round-trip correctness: sign with original, verify with recovered
// -----------------------------------------------------------------------

func TestMnemonicKey_SignOriginal_VerifyRecovered(t *testing.T) {
	t.Parallel()

	prikey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Sign with original key
	msg := []byte("sign-then-recover test")
	sig, err := SignByRSAPKCS1v15WithSHA256(prikey, msg)
	require.NoError(t, err)

	// Recover key from mnemonic
	mnemonic, err := PrikeyToMnemonic(prikey)
	require.NoError(t, err)

	recovered, err := MnemonicToPrikey(mnemonic)
	require.NoError(t, err)

	recoveredRSA := recovered.(*rsa.PrivateKey)

	// Verify with recovered key's public key
	err = VerifyByRSAPKCS1v15WithSHA256(&recoveredRSA.PublicKey, msg, sig)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------
// Ensure word count is reasonable
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_WordCount(t *testing.T) {
	t.Parallel()

	t.Run("Ed25519", func(t *testing.T) {
		_, prikey, _ := ed25519.GenerateKey(rand.Reader)
		m, err := PrikeyToMnemonic(prikey)
		require.NoError(t, err)
		words := strings.Fields(m)
		// Ed25519 PKCS8 DER ≈ 48 bytes → ~38-40 words
		require.True(t, len(words) > 30 && len(words) < 50,
			"expected 30-50 words for Ed25519, got %d", len(words))
	})

	t.Run("ECDSA-P256", func(t *testing.T) {
		prikey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		m, err := PrikeyToMnemonic(prikey)
		require.NoError(t, err)
		words := strings.Fields(m)
		// ECDSA P256 PKCS8 DER ≈ 138 bytes
		require.True(t, len(words) > 80 && len(words) < 120,
			"expected 80-120 words for ECDSA P256, got %d", len(words))
	})

	t.Run("RSA-2048", func(t *testing.T) {
		prikey, _ := rsa.GenerateKey(rand.Reader, 2048)
		m, err := PrikeyToMnemonic(prikey)
		require.NoError(t, err)
		words := strings.Fields(m)
		// RSA 2048 PKCS8 DER ≈ 1218 bytes
		require.True(t, len(words) > 800 && len(words) < 1000,
			"expected 800-1000 words for RSA 2048, got %d", len(words))
	})
}

// -----------------------------------------------------------------------
// Test with nil key
// -----------------------------------------------------------------------

func TestPrikeyToMnemonic_NilKey(t *testing.T) {
	t.Parallel()

	_, err := PrikeyToMnemonic(nil)
	require.Error(t, err)
}

// -----------------------------------------------------------------------
// Unused import guard (big.Int)
// -----------------------------------------------------------------------

var _ = new(big.Int)
