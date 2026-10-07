package crypto

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// PrikeyToMnemonic / MnemonicToPrikey
// -----------------------------------------------------------------------

// TestPrikeyToMnemonic_Ed25519 verifies that an Ed25519 private key encoded by PrikeyToMnemonic
// without a passphrase is recovered by MnemonicToPrikey as an equal ed25519.PrivateKey.
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

// TestPrikeyToMnemonic_ECDSA_P256 verifies that an ECDSA P-256 private key encoded by
// PrikeyToMnemonic without a passphrase is recovered by MnemonicToPrikey as an equal key.
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

// TestPrikeyToMnemonic_ECDSA_P384 verifies that an ECDSA P-384 private key encoded by
// PrikeyToMnemonic without a passphrase is recovered by MnemonicToPrikey as an equal key.
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

// TestPrikeyToMnemonic_ECDSA_P521 verifies that an ECDSA P-521 private key encoded by
// PrikeyToMnemonic without a passphrase is recovered by MnemonicToPrikey as an equal key.
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

// TestPrikeyToMnemonic_RSA2048 verifies that an RSA-2048 private key encoded by PrikeyToMnemonic
// without a passphrase is recovered by MnemonicToPrikey as an equal *rsa.PrivateKey.
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

// TestPrikeyToMnemonic_RSA4096 verifies that an RSA-4096 private key encoded by PrikeyToMnemonic
// without a passphrase is recovered by MnemonicToPrikey as an equal *rsa.PrivateKey.
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

// TestPrikeyToMnemonic_WithPassphrase_Ed25519 verifies that an Ed25519 key encrypted with
// WithMnemonicPassphrase is recovered intact by MnemonicToPrikey given the same passphrase.
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

// TestPrikeyToMnemonic_WithPassphrase_RSA verifies that an RSA-2048 key encrypted with
// WithMnemonicPassphrase is recovered intact by MnemonicToPrikey given the same passphrase.
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

// TestPrikeyToMnemonic_WithPassphrase_ECDSA verifies that an ECDSA P-256 key encrypted with
// WithMnemonicPassphrase is recovered intact by MnemonicToPrikey given the same passphrase.
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

// TestPrikeyToMnemonic_WrongPassphrase verifies that MnemonicToPrikey returns an error when a
// passphrase-encrypted mnemonic is decoded with a different passphrase than the one used to encode it.
func TestPrikeyToMnemonic_WrongPassphrase(t *testing.T) {
	t.Parallel()

	_, prikey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mnemonic, err := PrikeyToMnemonic(prikey, WithMnemonicPassphrase("correct"))
	require.NoError(t, err)

	_, err = MnemonicToPrikey(mnemonic, WithMnemonicPassphrase("wrong"))
	require.Error(t, err)
}

// TestPrikeyToMnemonic_NoPassphraseOnEncrypted verifies that MnemonicToPrikey refuses to decode a
// passphrase-encrypted mnemonic when no passphrase option is given, returning an "encrypted but no
// passphrase" error.
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

// TestWithMnemonicPassphrase_Empty verifies that applying WithMnemonicPassphrase with an empty
// passphrase fails with a "must not be empty" error.
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

// TestPrikeyToMnemonic_DifferentPassphrases verifies that encrypting the same Ed25519 key under two
// different passphrases produces two different mnemonics.
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

// TestPrikeyToMnemonic_EncryptedVsUnencrypted verifies that the plain and passphrase-encrypted
// mnemonics of the same Ed25519 key differ, yet both decode back to the same key when the matching
// options are supplied.
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
// Stability test: encoding is deterministic (no passphrase)
// -----------------------------------------------------------------------

// TestPrikeyToMnemonic_Deterministic verifies that, without a passphrase, encoding the same Ed25519
// key twice with PrikeyToMnemonic yields the identical mnemonic.
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
// Verify the encrypted marker byte detection
// -----------------------------------------------------------------------

// TestEncryptedMarkerDetection verifies that a payload whose first byte equals
// mnemonicEncryptedMarker round-trips unchanged through BytesToMnemonic and MnemonicToBytes, showing
// that the byte-level codec gives the marker no special meaning.
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
// Ensure word count is reasonable
// -----------------------------------------------------------------------

// TestPrikeyToMnemonic_WordCount verifies that unencrypted key mnemonics have plausible lengths:
// more than 30 and fewer than 50 words for Ed25519, more than 80 and fewer than 120 for ECDSA P-256,
// and more than 800 and fewer than 1000 for RSA-2048.
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

// TestPrikeyToMnemonic_NilKey verifies that PrikeyToMnemonic returns an error for a nil private key.
func TestPrikeyToMnemonic_NilKey(t *testing.T) {
	t.Parallel()

	_, err := PrikeyToMnemonic(nil)
	require.Error(t, err)
}
