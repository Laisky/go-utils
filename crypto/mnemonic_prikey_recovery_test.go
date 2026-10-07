package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// Signing round-trip: key recovered from mnemonic can sign/verify
// -----------------------------------------------------------------------

// TestMnemonicKey_CanSign_Ed25519 verifies that an Ed25519 key recovered from its mnemonic produces
// a signature that ed25519.Verify accepts under the original public key.
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

// TestMnemonicKey_CanSign_RSA verifies that an RSA-2048 key recovered from its mnemonic signs with
// SignByRSAPKCS1v15WithSHA256 and the signature verifies under the original public key.
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

// TestMnemonicKey_CanSign_ECDSA verifies that an ECDSA P-256 key recovered from its mnemonic signs
// with SignByECDSAWithSHA256 and the signature verifies under the original public key.
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

// TestMnemonicToPrikey_InvalidMnemonic verifies that MnemonicToPrikey rejects a three-word phrase
// that is too short to hold the extended-format header and checksum.
func TestMnemonicToPrikey_InvalidMnemonic(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToPrikey("abandon abandon abandon")
	require.Error(t, err)
}

// TestMnemonicToPrikey_EmptyMnemonic verifies that MnemonicToPrikey returns an error for an empty
// mnemonic.
func TestMnemonicToPrikey_EmptyMnemonic(t *testing.T) {
	t.Parallel()

	_, err := MnemonicToPrikey("")
	require.Error(t, err)
}

// BenchmarkPrikeyToMnemonic_Ed25519 measures PrikeyToMnemonic, without a passphrase, encoding one
// pre-generated Ed25519 key (PKCS#8 DER serialization plus extended mnemonic encoding).
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

// TestPrikeyToMnemonic_ECDSA_ScalarPreserved verifies that an ECDSA P-256 mnemonic round trip
// preserves the private scalar D and the public point coordinates X and Y exactly.
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

// TestPrikeyToMnemonic_RSA_ComponentsPreserved verifies that an RSA-2048 mnemonic round trip
// preserves the modulus N, public exponent E, private exponent D and every prime factor.
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

// TestPrikeyToMnemonic_Ed25519_SeedPreserved verifies that an Ed25519 mnemonic round trip preserves
// the private key seed and the derived public key.
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

// TestMnemonicKey_CrossValidate_SignByEd25519WithSHA512 verifies that an Ed25519 key recovered from
// its mnemonic works with SignByEd25519WithSHA512, and the signature verifies with
// VerifyByEd25519WithSHA512 under the original public key.
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

// TestMnemonicKey_SignOriginal_VerifyRecovered verifies that a signature made with the original
// RSA-2048 key via SignByRSAPKCS1v15WithSHA256 verifies under the public key of the key recovered
// from its mnemonic.
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
