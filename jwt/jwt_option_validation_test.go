package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/crypto"
)

func TestWithSecretByteValidation(t *testing.T) {
	t.Parallel()

	// Test that empty secret is rejected
	_, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("")),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret cannot be empty")

	// Test that nil secret is rejected
	_, err = New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte(nil),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret cannot be empty")

	_, err = New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("short-secret")),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret must be at least 32 bytes for HS256")

	// Test that valid secret works
	_, err = New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("12345678901234567890123456789012")),
	)
	require.NoError(t, err)
}

func TestWithPriKeyByteValidation(t *testing.T) {
	t.Parallel()

	// Test that empty private key is rejected
	_, err := New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte([]byte("")),
		WithPubKeyByte(es256PubByte),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private key cannot be empty")

	// Test that nil private key is rejected
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte(nil),
		WithPubKeyByte(es256PubByte),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private key cannot be empty")

	// Test that valid private key works
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte(es256PriByte),
		WithPubKeyByte(es256PubByte),
	)
	require.NoError(t, err)
}

func TestWithPubKeyByteValidation(t *testing.T) {
	t.Parallel()

	// Test that empty public key is rejected
	_, err := New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte(es256PriByte),
		WithPubKeyByte([]byte("")),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "public key cannot be empty")

	// Test that nil public key is rejected
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte(es256PriByte),
		WithPubKeyByte(nil),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "public key cannot be empty")

	// Test that valid public key works
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte(es256PriByte),
		WithPubKeyByte(es256PubByte),
	)
	require.NoError(t, err)
}

func TestDivideOptionValidation(t *testing.T) {
	t.Parallel()

	j, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte(secret),
	)
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	// Test WithDivideSecret validation
	_, err = j.SignByHS256(claims, WithDivideSecret([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide secret cannot be empty")

	_, err = j.SignByHS256(claims, WithDivideSecret(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide secret cannot be empty")

	_, err = j.SignByHS256(claims, WithDivideSecret([]byte("short-secret")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide secret must be at least 32 bytes for HS256")

	// Test WithDividePriKey validation
	_, err = j.SignByES256(claims, WithDividePriKey([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide private key cannot be empty")

	_, err = j.SignByES256(claims, WithDividePriKey(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide private key cannot be empty")

	// Test WithDividePubKey validation
	_, err = j.SignByES256(claims, WithDividePubKey([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide public key cannot be empty")

	_, err = j.SignByES256(claims, WithDividePubKey(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide public key cannot be empty")

	// Test that valid divide options work
	_, err = j.SignByHS256(claims, WithDivideSecret([]byte("12345678901234567890123456789012")))
	require.NoError(t, err)
}

func TestParseWithDivideOptionsOnly(t *testing.T) {
	t.Parallel()

	// Test that we can create JWT instance without main keys and use divide options
	// This should work for parsing tokens where keys are provided via divide options

	// First, create a token with a JWT that has keys
	j1, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("12345678901234567890123456789012")),
	)
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := j1.SignByHS256(claims)
	require.NoError(t, err)

	// Now test if we can parse it with a JWT instance that uses only divide options
	// This should work if the user wants to parse multiple tokens with different keys
	j2, err := New(WithSignMethod(SignMethodHS256))
	require.NoError(t, err) // This should work - no keys required at creation

	// Parse using divide options
	parsedClaims := &testJWTClaims{}
	err = j2.ParseClaimsByHS256(token, parsedClaims, WithDivideSecret([]byte("12345678901234567890123456789012")))
	require.NoError(t, err)
	require.Equal(t, "test-user", parsedClaims.Subject)
}

func TestParseWithoutKeysFailsGracefully(t *testing.T) {
	t.Parallel()

	// Test that parsing without any keys fails gracefully with a meaningful error

	// Create a token first
	j1, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("12345678901234567890123456789012")),
	)
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := j1.SignByHS256(claims)
	require.NoError(t, err)

	// Create JWT instance without any keys
	j2, err := New(WithSignMethod(SignMethodHS256))
	require.NoError(t, err)

	// Try to parse without providing any keys - this should fail but not panic
	parsedClaims := &testJWTClaims{}
	err = j2.ParseClaimsByHS256(token, parsedClaims)
	require.Error(t, err) // Should fail because no secret is provided
	require.Contains(t, err.Error(), "HS256 secret must not be empty")
}

func TestRS256ParsingValidation(t *testing.T) {
	t.Parallel()

	// Generate RSA keys using crypto utilities
	rsaPrivateKey, err := crypto.NewRSAPrikey(crypto.RSAPrikeyBits2048)
	require.NoError(t, err)

	rsaPublicKeyPEM, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(rsaPrivateKey))
	require.NoError(t, err)

	// Test RS256 validation works with empty keys
	j, err := New(WithSignMethod(SignMethodRS256))
	require.NoError(t, err)

	// Test parsing with empty divide public key fails
	claims := &testJWTClaims{}
	err = j.ParseClaimsByRS256("dummy.jwt.token", claims, WithDividePubKey([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide public key cannot be empty")

	// Test parsing with nil divide public key fails
	err = j.ParseClaimsByRS256("dummy.jwt.token", claims, WithDividePubKey(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide public key cannot be empty")

	// Test parsing with empty divide private key fails
	err = j.ParseClaimsByRS256("dummy.jwt.token", claims, WithDividePriKey([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide private key cannot be empty")

	// Test parsing with nil divide private key fails
	err = j.ParseClaimsByRS256("dummy.jwt.token", claims, WithDividePriKey(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide private key cannot be empty")

	// Test that valid keys don't cause validation errors (even if parsing might fail for other reasons)
	err = j.ParseClaimsByRS256("dummy.jwt.token", claims, WithDividePubKey(rsaPublicKeyPEM))
	// This might fail due to invalid token format, but NOT due to our validation
	if err != nil {
		require.NotContains(t, err.Error(), "divide public key cannot be empty")
		require.NotContains(t, err.Error(), "divide private key cannot be empty")
	}
}

func TestMixedValidationScenarios(t *testing.T) {
	t.Parallel()

	// Test combinations of valid and invalid options

	// Test valid secret with invalid divide secret
	j, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("12345678901234567890123456789012")),
	)
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	// This should fail due to empty divide secret, even though main secret is valid
	_, err = j.SignByHS256(claims, WithDivideSecret([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide secret cannot be empty")

	// Test multiple invalid options in sequence
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte([]byte("")), // This should fail first
		WithPubKeyByte(es256PubByte),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "private key cannot be empty")

	// Test both keys empty
	_, err = New(
		WithSignMethod(SignMethodES256),
		WithPriKeyByte([]byte("")),
		WithPubKeyByte([]byte("")),
	)
	require.Error(t, err)
	// Should fail on the first empty check
	require.Contains(t, err.Error(), "private key cannot be empty")
}
