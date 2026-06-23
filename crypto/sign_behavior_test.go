package crypto

import (
	"bytes"
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSignBehavior_RSAReader tests SignReaderByRSAWithSHA256 / VerifyReaderByRSAWithSHA256.
func TestSignBehavior_RSAReader(t *testing.T) {
	t.Parallel()

	prikey, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)

	prikey2, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)

	content := []byte("hello rsa reader signing")

	t.Run("sign and verify with correct key", func(t *testing.T) {
		t.Parallel()
		sig, err := SignReaderByRSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		err = VerifyReaderByRSAWithSHA256(&prikey.PublicKey, bytes.NewReader(content), sig)
		require.NoError(t, err)
	})

	t.Run("verify fails with wrong key", func(t *testing.T) {
		t.Parallel()
		sig, err := SignReaderByRSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)

		err = VerifyReaderByRSAWithSHA256(&prikey2.PublicKey, bytes.NewReader(content), sig)
		require.Error(t, err)
	})

	t.Run("verify fails with wrong content", func(t *testing.T) {
		t.Parallel()
		sig, err := SignReaderByRSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)

		err = VerifyReaderByRSAWithSHA256(&prikey.PublicKey, bytes.NewReader([]byte("tampered")), sig)
		require.Error(t, err)
	})
}

// TestSignBehavior_ECDSAReader tests SignReaderByECDSAWithSHA256 / VerifyReaderByECDSAWithSHA256.
func TestSignBehavior_ECDSAReader(t *testing.T) {
	t.Parallel()

	prikey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	prikey2, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	content := []byte("hello ecdsa reader signing")

	t.Run("sign and verify with correct key", func(t *testing.T) {
		t.Parallel()
		r, s, err := SignReaderByECDSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)
		require.NotNil(t, r)
		require.NotNil(t, s)

		ok, err := VerifyReaderByECDSAWithSHA256(&prikey.PublicKey, bytes.NewReader(content), r, s)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("verify fails with wrong key", func(t *testing.T) {
		t.Parallel()
		r, s, err := SignReaderByECDSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)

		ok, err := VerifyReaderByECDSAWithSHA256(&prikey2.PublicKey, bytes.NewReader(content), r, s)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("verify fails with wrong content", func(t *testing.T) {
		t.Parallel()
		r, s, err := SignReaderByECDSAWithSHA256(prikey, bytes.NewReader(content))
		require.NoError(t, err)

		ok, err := VerifyReaderByECDSAWithSHA256(&prikey.PublicKey, bytes.NewReader([]byte("tampered")), r, s)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// TestSignBehavior_ECDSABase64 tests SignByECDSAWithSHA256AndBase64 / VerifyByECDSAWithSHA256AndBase64.
func TestSignBehavior_ECDSABase64(t *testing.T) {
	t.Parallel()

	prikey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	prikey2, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	content := []byte("hello ecdsa base64 signing")

	t.Run("sign and verify with correct key", func(t *testing.T) {
		t.Parallel()
		sig, err := SignByECDSAWithSHA256AndBase64(prikey, content)
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		ok, err := VerifyByECDSAWithSHA256AndBase64(&prikey.PublicKey, content, sig)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("verify fails with wrong key", func(t *testing.T) {
		t.Parallel()
		sig, err := SignByECDSAWithSHA256AndBase64(prikey, content)
		require.NoError(t, err)

		ok, err := VerifyByECDSAWithSHA256AndBase64(&prikey2.PublicKey, content, sig)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("verify fails with wrong content", func(t *testing.T) {
		t.Parallel()
		sig, err := SignByECDSAWithSHA256AndBase64(prikey, content)
		require.NoError(t, err)

		ok, err := VerifyByECDSAWithSHA256AndBase64(&prikey.PublicKey, []byte("tampered"), sig)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("non-deterministic signatures", func(t *testing.T) {
		t.Parallel()
		sig1, err := SignByECDSAWithSHA256AndBase64(prikey, content)
		require.NoError(t, err)

		sig2, err := SignByECDSAWithSHA256AndBase64(prikey, content)
		require.NoError(t, err)

		require.NotEqual(t, sig1, sig2, "ECDSA signatures should be non-deterministic")
	})
}

// TestSignBehavior_DecodeES256SignByHex_Errors tests error cases for DecodeES256SignByHex.
func TestSignBehavior_DecodeES256SignByHex_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"no delimiter", "abcdef1234567890"},
		{"too many delimiters", "ab.cd.ef"},
		{"invalid hex in r", "ZZZZ.abcd"},
		{"invalid hex in s", "abcd.ZZZZ"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := DecodeES256SignByHex(tc.input)
			require.Error(t, err)
		})
	}
}

// TestSignBehavior_DecodeES256SignByBase64_Errors tests error cases for DecodeES256SignByBase64.
func TestSignBehavior_DecodeES256SignByBase64_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid base64 in r", func(t *testing.T) {
		t.Parallel()
		_, _, err := DecodeES256SignByBase64("!!!invalid!!!.dGVzdA==")
		require.Error(t, err)
	})

	t.Run("invalid base64 in s", func(t *testing.T) {
		t.Parallel()
		_, _, err := DecodeES256SignByBase64("dGVzdA==.!!!invalid!!!")
		require.Error(t, err)
	})

	// Regression: DecodeES256SignByBase64 must return a real error (not nil) for
	// format issues. Previously it wrapped a nil error, silently returning
	// (nil, nil, nil), which caused a downstream nil-pointer panic in
	// VerifyByECDSAWithSHA256AndBase64. See fix in sign.go.
	t.Run("no delimiter returns error", func(t *testing.T) {
		t.Parallel()
		r, s, err := DecodeES256SignByBase64("dGVzdA==dGVzdA==")
		require.Error(t, err, "malformed signature must produce an error, not nil")
		require.Nil(t, r)
		require.Nil(t, s)
	})

	t.Run("too many delimiters returns error", func(t *testing.T) {
		t.Parallel()
		r, s, err := DecodeES256SignByBase64("dGVzdA==.dGVzdA==.dGVzdA==")
		require.Error(t, err, "malformed signature must produce an error, not nil")
		require.Nil(t, r)
		require.Nil(t, s)
	})
}

// TestSignBehavior_EncodeDecodeES256Hex_Roundtrip tests hex encode/decode roundtrip.
func TestSignBehavior_EncodeDecodeES256Hex_Roundtrip(t *testing.T) {
	t.Parallel()

	r := big.NewInt(123456789)
	s := big.NewInt(987654321)

	encoded := EncodeES256SignByHex(r, s)
	require.Contains(t, encoded, ".")

	decodedR, decodedS, err := DecodeES256SignByHex(encoded)
	require.NoError(t, err)
	require.Equal(t, 0, r.Cmp(decodedR), "r values should match after roundtrip")
	require.Equal(t, 0, s.Cmp(decodedS), "s values should match after roundtrip")
}

// TestSignBehavior_EncodeDecodeES256Base64_Roundtrip tests base64 encode/decode roundtrip.
func TestSignBehavior_EncodeDecodeES256Base64_Roundtrip(t *testing.T) {
	t.Parallel()

	r := big.NewInt(123456789)
	s := big.NewInt(987654321)

	encoded := EncodeES256SignByBase64(r, s)
	require.Contains(t, encoded, ".")

	decodedR, decodedS, err := DecodeES256SignByBase64(encoded)
	require.NoError(t, err)
	require.Equal(t, 0, r.Cmp(decodedR), "r values should match after roundtrip")
	require.Equal(t, 0, s.Cmp(decodedS), "s values should match after roundtrip")
}

// TestSignBehavior_HMACSha256_Determinism tests that HMAC is deterministic and key-sensitive.
func TestSignBehavior_HMACSha256_Determinism(t *testing.T) {
	t.Parallel()

	key1 := []byte("secret-key-1")
	key2 := []byte("secret-key-2")
	data := []byte("data to hash")

	t.Run("same key and data produce same HMAC", func(t *testing.T) {
		t.Parallel()
		mac1, err := HMACSha256(key1, bytes.NewReader(data))
		require.NoError(t, err)

		mac2, err := HMACSha256(key1, bytes.NewReader(data))
		require.NoError(t, err)

		require.Equal(t, mac1, mac2)
	})

	t.Run("different key produces different HMAC", func(t *testing.T) {
		t.Parallel()
		mac1, err := HMACSha256(key1, bytes.NewReader(data))
		require.NoError(t, err)

		mac2, err := HMACSha256(key2, bytes.NewReader(data))
		require.NoError(t, err)

		require.NotEqual(t, mac1, mac2)
	})
}

// TestSignBehavior_Ed25519WithSHA512_Failures tests Ed25519 verification failure cases.
func TestSignBehavior_Ed25519WithSHA512_Failures(t *testing.T) {
	t.Parallel()

	prikey, err := NewEd25519Prikey()
	require.NoError(t, err)

	prikey2, err := NewEd25519Prikey()
	require.NoError(t, err)

	content := []byte("hello ed25519 signing")

	sig, err := SignByEd25519WithSHA512(prikey, bytes.NewReader(content))
	require.NoError(t, err)

	pubkey := prikey.Public().(ed25519.PublicKey)
	pubkey2 := prikey2.Public().(ed25519.PublicKey)

	t.Run("verify with wrong pubkey should fail", func(t *testing.T) {
		t.Parallel()
		err := VerifyByEd25519WithSHA512(pubkey2, bytes.NewReader(content), sig)
		require.Error(t, err)
	})

	t.Run("verify with tampered content should fail", func(t *testing.T) {
		t.Parallel()
		err := VerifyByEd25519WithSHA512(pubkey, bytes.NewReader([]byte("tampered")), sig)
		require.Error(t, err)
	})
}
