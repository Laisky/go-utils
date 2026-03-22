package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// TestCryptoBehavior_PasswordHashUnsupportedHasher verifies that PasswordHash
// rejects unsupported hash types with the expected error message.
func TestCryptoBehavior_PasswordHashUnsupportedHasher(t *testing.T) {
	t.Parallel()
	_, err := PasswordHash([]byte("secret"), gutils.HashType("md5"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "only supprt sha256,sha512")
}

// TestCryptoBehavior_ParseHashedPasswordMalformed tests that parseHashedPassword
// correctly rejects various malformed inputs.
func TestCryptoBehavior_ParseHashedPasswordMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		errAs string
	}{
		{
			name:  "wrong number of parts - too few",
			input: "sha256.10000.abcd",
			errAs: "4 parts",
		},
		{
			name:  "wrong number of parts - too many",
			input: "sha256.10000.abcd.abcd.extra",
			errAs: "4 parts",
		},
		{
			name:  "non-numeric hashNum",
			input: "sha256.abc.abcd.abcd",
			errAs: "parse hash num",
		},
		{
			name:  "invalid hex salt",
			input: "sha256.10000.ZZZZ.abcd",
			errAs: "decode salt",
		},
		{
			name:  "invalid hex password",
			input: "sha256.10000.abcd.ZZZZ",
			errAs: "decode hashed password",
		},
		{
			name:  "zero iterations",
			input: "sha256.0.abcd.abcd",
			errAs: "invalid iterations",
		},
		{
			name:  "negative iterations",
			input: "sha256.-1.abcd.abcd",
			errAs: "invalid iterations",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseHashedPassword(tc.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errAs)
		})
	}
}

// TestCryptoBehavior_VerifyHashedPasswordWrongPassword confirms that a correct
// hash verified against the wrong password returns "password not match".
func TestCryptoBehavior_VerifyHashedPasswordWrongPassword(t *testing.T) {
	t.Parallel()

	// Build a valid hashed password string directly to avoid the 2s delay of PasswordHash.
	salt := []byte("saltsalt")
	hp, err := newHashedPasswordWithMinIteration(salt, []byte("correctpassword"), gutils.HashTypeSha256, 10000, 1)
	require.NoError(t, err)
	hpStr := hp.String()

	err = VerifyHashedPassword([]byte("wrongpassword"), hpStr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "password not match")
}

// TestCryptoBehavior_RSAEncryptPKCS1v15WrongKey verifies that encrypting with
// one key pair and decrypting with another fails.
func TestCryptoBehavior_RSAEncryptPKCS1v15WrongKey(t *testing.T) {
	t.Parallel()

	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyB, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	plaintext := []byte("hello world")
	cipher, err := RSAEncryptByPKCS1v15(&keyA.PublicKey, plaintext)
	require.NoError(t, err)

	_, err = RSADecryptByPKCS1v15(keyB, cipher)
	require.Error(t, err)
}

// TestCryptoBehavior_RSAEncryptPKCS1v15NonDeterministic verifies that
// encrypting the same plaintext twice produces different ciphertexts.
func TestCryptoBehavior_RSAEncryptPKCS1v15NonDeterministic(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	plaintext := []byte("deterministic check")
	c1, err := RSAEncryptByPKCS1v15(&key.PublicKey, plaintext)
	require.NoError(t, err)
	c2, err := RSAEncryptByPKCS1v15(&key.PublicKey, plaintext)
	require.NoError(t, err)

	require.NotEqual(t, c1, c2, "PKCS1v15 encryption should be non-deterministic")
}

// TestCryptoBehavior_FormatBig2HexRoundtrip tests hex encoding/decoding roundtrip
// for zero and large values.
func TestCryptoBehavior_FormatBig2HexRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  *big.Int
	}{
		{"zero", big.NewInt(0)},
		{"one", big.NewInt(1)},
		{"small", big.NewInt(255)},
		{"large", new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hexStr := FormatBig2Hex(tc.val)
			got, ok := ParseHex2Big(hexStr)
			require.True(t, ok)
			require.Equal(t, 0, tc.val.Cmp(got), "roundtrip mismatch for %s", tc.name)
		})
	}
}

// TestCryptoBehavior_FormatBig2Base64Roundtrip tests base64 encoding/decoding roundtrip.
func TestCryptoBehavior_FormatBig2Base64Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  *big.Int
	}{
		{"one", big.NewInt(1)},
		{"small", big.NewInt(255)},
		{"large", new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b64 := FormatBig2Base64(tc.val)
			got, err := ParseBase642Big(b64)
			require.NoError(t, err)
			require.Equal(t, 0, tc.val.Cmp(got), "roundtrip mismatch for %s", tc.name)
		})
	}
}

// TestCryptoBehavior_ParseHex2BigInvalid verifies that invalid hex returns ok=false.
func TestCryptoBehavior_ParseHex2BigInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"contains g", "abcg"},
		{"spaces", "ab cd"},
		{"special chars", "!@#$"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ok := ParseHex2Big(tc.input)
			require.False(t, ok)
		})
	}
}

// TestCryptoBehavior_ParseBase642BigInvalid verifies that invalid base64 returns an error.
func TestCryptoBehavior_ParseBase642BigInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"bad padding", "!!!"},
		{"invalid chars", "abc{def}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseBase642Big(tc.input)
			require.Error(t, err)
		})
	}
}

// TestCryptoBehavior_HashedPasswordStringFormat verifies the output format
// of HashedPassword.String() is "hasher.hashNum.hexSalt.hexPassword".
func TestCryptoBehavior_HashedPasswordStringFormat(t *testing.T) {
	t.Parallel()

	salt := []byte("testsalt")
	hp, err := newHashedPasswordWithMinIteration(salt, []byte("mypassword"), gutils.HashTypeSha256, 10000, 1)
	require.NoError(t, err)

	s := hp.String()
	parts := strings.Split(s, ".")
	require.Len(t, parts, 4, "HashedPassword.String() must have 4 dot-separated parts")
	require.Equal(t, "sha256", parts[0])
	require.Equal(t, "10000", parts[1])
	// parts[2] and parts[3] should be valid hex
	require.NotEmpty(t, parts[2])
	require.NotEmpty(t, parts[3])
}
