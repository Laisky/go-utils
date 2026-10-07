package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================
// Key Exchange behavioral tests
// ============================================================

// TestExchangeHkdfBehavior_ECDHCrossCurveMismatch verifies that a P-256 ECDH instance fails to
// derive a shared key from a P-384 peer public key.
func TestExchangeHkdfBehavior_ECDHCrossCurveMismatch(t *testing.T) {
	t.Parallel()

	alice, err := NewEcdh(ECDSACurveP256)
	require.NoError(t, err)

	bob, err := NewEcdh(ECDSACurveP384)
	require.NoError(t, err)

	bobPub, err := bob.PublicKey()
	require.NoError(t, err)

	_, err = alice.GenerateKey(bobPub)
	require.Error(t, err, "cross-curve ECDH should fail")
}

// TestExchangeHkdfBehavior_ECDHKeyUniqueness verifies that two ECDH instances created on the same
// curve have different public keys, for each of P-256, P-384 and P-521.
func TestExchangeHkdfBehavior_ECDHKeyUniqueness(t *testing.T) {
	t.Parallel()

	curves := []ECDSACurve{ECDSACurveP256, ECDSACurveP384, ECDSACurveP521}
	for _, curve := range curves {
		curve := curve
		t.Run(string(curve), func(t *testing.T) {
			t.Parallel()

			a, err := NewEcdh(curve)
			require.NoError(t, err)
			b, err := NewEcdh(curve)
			require.NoError(t, err)

			aPub, err := a.PublicKey()
			require.NoError(t, err)
			bPub, err := b.PublicKey()
			require.NoError(t, err)

			require.False(t, bytes.Equal(aPub, bPub),
				"two ECDH instances on the same curve should have different public keys")
		})
	}
}

// TestExchangeHkdfBehavior_ECDHInvalidPeerPublicKey verifies that ECDH.GenerateKey rejects a peer
// public key that carries the P-256 curve tag followed by bytes that do not encode a valid point.
func TestExchangeHkdfBehavior_ECDHInvalidPeerPublicKey(t *testing.T) {
	t.Parallel()

	alice, err := NewEcdh(ECDSACurveP256)
	require.NoError(t, err)

	// byte 1 => P256 curve tag, followed by garbage
	_, err = alice.GenerateKey([]byte{1, 0xFF, 0xFF})
	require.Error(t, err, "garbage peer public key should be rejected")
}

// TestExchangeHkdfBehavior_DHKXDeterminism verifies that repeated DHKX.GenerateKey calls with the
// same peer public key return the same shared key, and that this key equals the one the peer
// derives in the opposite direction.
func TestExchangeHkdfBehavior_DHKXDeterminism(t *testing.T) {
	t.Parallel()

	alice, err := NewDHKX()
	require.NoError(t, err)
	bob, err := NewDHKX()
	require.NoError(t, err)

	alicePub, err := alice.PublicKey()
	require.NoError(t, err)
	bobPub, err := bob.PublicKey()
	require.NoError(t, err)

	key1, err := alice.GenerateKey(bobPub)
	require.NoError(t, err)
	key2, err := alice.GenerateKey(bobPub)
	require.NoError(t, err)
	require.True(t, bytes.Equal(key1, key2),
		"same DHKX pair should always produce the same shared key")

	bobKey, err := bob.GenerateKey(alicePub)
	require.NoError(t, err)
	require.True(t, bytes.Equal(key1, bobKey),
		"DHKX shared key must be symmetric")
}

// TestExchangeHkdfBehavior_ECDHSharedKeySymmetry verifies that on the P-384 and P-521 curves both
// ECDH peers derive the same shared key from each other's public keys.
func TestExchangeHkdfBehavior_ECDHSharedKeySymmetry(t *testing.T) {
	t.Parallel()

	curves := []ECDSACurve{ECDSACurveP384, ECDSACurveP521}
	for _, curve := range curves {
		curve := curve
		t.Run(string(curve), func(t *testing.T) {
			t.Parallel()

			alice, err := NewEcdh(curve)
			require.NoError(t, err)
			bob, err := NewEcdh(curve)
			require.NoError(t, err)

			alicePub, err := alice.PublicKey()
			require.NoError(t, err)
			bobPub, err := bob.PublicKey()
			require.NoError(t, err)

			aliceShared, err := alice.GenerateKey(bobPub)
			require.NoError(t, err)
			bobShared, err := bob.GenerateKey(alicePub)
			require.NoError(t, err)

			require.True(t, bytes.Equal(aliceShared, bobShared),
				"alice and bob should derive the same shared key")
		})
	}
}

// ============================================================
// HKDF behavioral tests
// ============================================================

// TestExchangeHkdfBehavior_HKDFWithInfo verifies that HKDFWithSHA256 with a fixed secret and salt
// derives different 32-byte keys for different info values and the same key for the same info
// value.
func TestExchangeHkdfBehavior_HKDFWithInfo(t *testing.T) {
	t.Parallel()

	secret := []byte("shared-secret")
	salt := []byte("some-salt-value!")

	derive := func(info []byte) []byte {
		results := [][]byte{make([]byte, 32)}
		err := HKDFWithSHA256(secret, salt, info, results)
		require.NoError(t, err)
		return results[0]
	}

	k1 := derive([]byte("context-a"))
	k2 := derive([]byte("context-b"))
	k3 := derive([]byte("context-a")) // same as k1

	require.False(t, bytes.Equal(k1, k2),
		"different info values should produce different derived keys")
	require.True(t, bytes.Equal(k1, k3),
		"same info value should produce the same derived key")
}

// TestExchangeHkdfBehavior_HKDFDifferentSalts verifies that HKDFWithSHA256 with a fixed secret and
// info derives different 32-byte keys for two different salts.
func TestExchangeHkdfBehavior_HKDFDifferentSalts(t *testing.T) {
	t.Parallel()

	secret := []byte("shared-secret")
	info := []byte("info")

	derive := func(salt []byte) []byte {
		results := [][]byte{make([]byte, 32)}
		err := HKDFWithSHA256(secret, salt, info, results)
		require.NoError(t, err)
		return results[0]
	}

	k1 := derive([]byte("salt-one-padding!"))
	k2 := derive([]byte("salt-two-padding!"))

	require.False(t, bytes.Equal(k1, k2),
		"different salts should produce different derived keys")
}

// TestExchangeHkdfBehavior_HKDFMultipleKeys verifies that filling five 32-byte result buffers in a
// single HKDFWithSHA256 call yields pairwise different keys.
func TestExchangeHkdfBehavior_HKDFMultipleKeys(t *testing.T) {
	t.Parallel()

	secret := []byte("shared-secret")
	salt := []byte("salt-value-here!")

	const n = 5
	results := make([][]byte, n)
	for i := range results {
		results[i] = make([]byte, 32)
	}

	err := HKDFWithSHA256(secret, salt, nil, results)
	require.NoError(t, err)

	// verify all keys are pairwise different
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			require.False(t, bytes.Equal(results[i], results[j]),
				"derived keys %d and %d should differ", i, j)
		}
	}
}

// TestExchangeHkdfBehavior_SaltUniqueness verifies that two Salt(32) calls return different random
// values.
func TestExchangeHkdfBehavior_SaltUniqueness(t *testing.T) {
	t.Parallel()

	s1, err := Salt(32)
	require.NoError(t, err)
	s2, err := Salt(32)
	require.NoError(t, err)

	require.False(t, bytes.Equal(s1, s2),
		"two Salt calls with same length should produce different values")
}

// TestExchangeHkdfBehavior_SaltVariousLengths verifies that Salt returns exactly the requested
// number of bytes for lengths 1, 32 and 1024.
func TestExchangeHkdfBehavior_SaltVariousLengths(t *testing.T) {
	t.Parallel()

	lengths := []int{1, 32, 1024}
	for _, length := range lengths {
		length := length
		t.Run("", func(t *testing.T) {
			t.Parallel()

			s, err := Salt(length)
			require.NoError(t, err)
			require.Len(t, s, length)
		})
	}
}

// TestExchangeHkdfBehavior_DeriveKeyByHKDFLengths verifies that DeriveKeyByHKDF returns a key of
// exactly the requested length for 1, 16, 32 and 64 bytes.
func TestExchangeHkdfBehavior_DeriveKeyByHKDFLengths(t *testing.T) {
	t.Parallel()

	rawKey := []byte("raw-key-material")
	salt := []byte("salt-value-here!")

	lengths := []int{1, 16, 32, 64}
	for _, length := range lengths {
		length := length
		t.Run("", func(t *testing.T) {
			t.Parallel()

			key, err := DeriveKeyByHKDF(rawKey, salt, length)
			require.NoError(t, err)
			require.Len(t, key, length)
		})
	}
}

// TestExchangeHkdfBehavior_DeriveKeyBySMHFDifferentInputs verifies that DeriveKeyBySMHF (scrypt)
// derives different keys from two different raw keys under the same salt.
func TestExchangeHkdfBehavior_DeriveKeyBySMHFDifferentInputs(t *testing.T) {
	t.Parallel()

	salt := []byte("salt-value-here!")

	k1, err := DeriveKeyBySMHF([]byte("key-alpha"), salt)
	require.NoError(t, err)
	k2, err := DeriveKeyBySMHF([]byte("key-bravo"), salt)
	require.NoError(t, err)

	require.False(t, bytes.Equal(k1, k2),
		"different raw keys should produce different derived keys")
}
