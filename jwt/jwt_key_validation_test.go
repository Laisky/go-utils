package jwt

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/crypto"
)

// TestValidationWithAllSigningMethods runs table-driven HS256 and ES256 cases checking that New accepts valid key
// material and rejects an empty secret, private key or public key with the matching error message.
func TestValidationWithAllSigningMethods(t *testing.T) {
	t.Parallel()

	// Test validation works consistently across all signing methods

	testCases := []struct {
		name           string
		signingMethod  jwt.SigningMethod
		validOptions   []Option
		invalidOptions []Option
		expectedError  string
	}{
		{
			name:           "HS256 with valid secret",
			signingMethod:  SignMethodHS256,
			validOptions:   []Option{WithSecretByte([]byte("12345678901234567890123456789012"))},
			invalidOptions: []Option{WithSecretByte([]byte(""))},
			expectedError:  "secret cannot be empty",
		},
		{
			name:           "ES256 with valid keys",
			signingMethod:  SignMethodES256,
			validOptions:   []Option{WithPriKeyByte(es256PriByte), WithPubKeyByte(es256PubByte)},
			invalidOptions: []Option{WithPriKeyByte([]byte("")), WithPubKeyByte(es256PubByte)},
			expectedError:  "private key cannot be empty",
		},
		{
			name:           "ES256 with invalid public key",
			signingMethod:  SignMethodES256,
			validOptions:   []Option{WithPriKeyByte(es256PriByte), WithPubKeyByte(es256PubByte)},
			invalidOptions: []Option{WithPriKeyByte(es256PriByte), WithPubKeyByte([]byte(""))},
			expectedError:  "public key cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test valid options work
			validOpts := append([]Option{WithSignMethod(tc.signingMethod)}, tc.validOptions...)
			_, err := New(validOpts...)
			require.NoError(t, err, "Valid options should not produce error")

			// Test invalid options fail
			invalidOpts := append([]Option{WithSignMethod(tc.signingMethod)}, tc.invalidOptions...)
			_, err = New(invalidOpts...)
			require.Error(t, err, "Invalid options should produce error")
			require.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

// TestComprehensiveKeyValidationWithGeneratedKeys verifies key validation with freshly generated keys: RS256 accepts a
// generated RSA PEM pair and rejects either key when empty, ES256 signs and parses a token round trip with a generated
// P-256 pair, and generated Ed25519 keys encode to PEM while New still rejects an empty private key.
func TestComprehensiveKeyValidationWithGeneratedKeys(t *testing.T) {
	t.Parallel()

	// Test with generated RSA keys
	t.Run("RSA Keys", func(t *testing.T) {
		t.Parallel()

		rsaPrivateKey, err := crypto.NewRSAPrikey(crypto.RSAPrikeyBits2048)
		require.NoError(t, err)

		rsaPrivateKeyPEM, err := crypto.Prikey2Pem(rsaPrivateKey)
		require.NoError(t, err)

		rsaPublicKeyPEM, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(rsaPrivateKey))
		require.NoError(t, err)

		// Test that valid RSA keys work for creation
		j, err := New(
			WithSignMethod(SignMethodRS256),
			WithPriKeyByte(rsaPrivateKeyPEM),
			WithPubKeyByte(rsaPublicKeyPEM),
		)
		require.NoError(t, err)
		require.NotNil(t, j)

		// Test validation still works with generated keys
		_, err = New(
			WithSignMethod(SignMethodRS256),
			WithPriKeyByte([]byte("")), // Empty should fail
			WithPubKeyByte(rsaPublicKeyPEM),
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "private key cannot be empty")

		_, err = New(
			WithSignMethod(SignMethodRS256),
			WithPriKeyByte(rsaPrivateKeyPEM),
			WithPubKeyByte([]byte("")), // Empty should fail
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "public key cannot be empty")
	})

	// Test with generated ECDSA keys
	t.Run("ECDSA Keys", func(t *testing.T) {
		t.Parallel()

		ecdsaPrivateKey, err := crypto.NewECDSAPrikey(crypto.ECDSACurveP256)
		require.NoError(t, err)

		ecdsaPrivateKeyPEM, err := crypto.Prikey2Pem(ecdsaPrivateKey)
		require.NoError(t, err)

		ecdsaPublicKeyPEM, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(ecdsaPrivateKey))
		require.NoError(t, err)

		// Test that valid ECDSA keys work for creation
		j, err := New(
			WithSignMethod(SignMethodES256),
			WithPriKeyByte(ecdsaPrivateKeyPEM),
			WithPubKeyByte(ecdsaPublicKeyPEM),
		)
		require.NoError(t, err)
		require.NotNil(t, j)

		// Test signing and parsing with generated keys
		claims := &testJWTClaims{
			jwt.RegisteredClaims{
				Subject:   "test-generated-keys",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		token, err := j.SignByES256(claims)
		require.NoError(t, err)
		require.NotEmpty(t, token)

		// Parse the token back
		parsedClaims := &testJWTClaims{}
		err = j.ParseClaimsByES256(token, parsedClaims)
		require.NoError(t, err)
		require.Equal(t, "test-generated-keys", parsedClaims.Subject)
	})

	// Test with Ed25519 keys (if supported for other crypto operations)
	t.Run("Ed25519 Keys", func(t *testing.T) {
		t.Parallel()

		ed25519PrivateKey, err := crypto.NewEd25519Prikey()
		require.NoError(t, err)

		ed25519PrivateKeyPEM, err := crypto.Prikey2Pem(ed25519PrivateKey)
		require.NoError(t, err)

		ed25519PublicKeyPEM, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(ed25519PrivateKey))
		require.NoError(t, err)

		// Test that Ed25519 keys can be validated (even if not directly used in JWT)
		require.NotEmpty(t, ed25519PrivateKeyPEM)
		require.NotEmpty(t, ed25519PublicKeyPEM)

		// Test empty key validation still works
		require.Error(t, func() error {
			_, err := New(WithPriKeyByte([]byte("")))
			return err
		}())
	})
}

// TestDivideOptionsWithGeneratedKeys verifies that a keyless RS256 instance accepts two different generated RSA public
// keys through WithDividePubKey without a key validation error, and still rejects an empty divide public key.
func TestDivideOptionsWithGeneratedKeys(t *testing.T) {
	t.Parallel()

	// Test divide options with dynamically generated keys

	// Generate multiple RSA key pairs for testing divide options
	rsaKey1, err := crypto.NewRSAPrikey(crypto.RSAPrikeyBits2048)
	require.NoError(t, err)

	rsaPublicKeyPEM1, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(rsaKey1))
	require.NoError(t, err)

	rsaKey2, err := crypto.NewRSAPrikey(crypto.RSAPrikeyBits2048)
	require.NoError(t, err)

	rsaPublicKeyPEM2, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(rsaKey2))
	require.NoError(t, err)

	// Create JWT instance without main keys
	j, err := New(WithSignMethod(SignMethodRS256))
	require.NoError(t, err)

	// Test that divide options with generated keys work
	claims := &testJWTClaims{}

	// These should pass validation (though parsing a dummy token will fail for other reasons)
	err = j.ParseClaimsByRS256("dummy.token", claims, WithDividePubKey(rsaPublicKeyPEM1))
	if err != nil {
		require.NotContains(t, err.Error(), "divide public key cannot be empty")
	}

	err = j.ParseClaimsByRS256("dummy.token", claims, WithDividePubKey(rsaPublicKeyPEM2))
	if err != nil {
		require.NotContains(t, err.Error(), "divide public key cannot be empty")
	}

	// Test that empty divide keys still fail validation
	err = j.ParseClaimsByRS256("dummy.token", claims, WithDividePubKey([]byte("")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "divide public key cannot be empty")
}

// TestKeyValidationWithDifferentKeySizes verifies, for 2048, 3072 and 4096-bit RSA keys, that New accepts a generated
// RS256 PEM key pair and rejects an empty private key.
func TestKeyValidationWithDifferentKeySizes(t *testing.T) {
	t.Parallel()

	// Test validation works with different RSA key sizes
	keySizes := []crypto.RSAPrikeyBits{
		crypto.RSAPrikeyBits2048,
		crypto.RSAPrikeyBits3072,
		crypto.RSAPrikeyBits4096,
	}

	for _, keySize := range keySizes {
		t.Run(fmt.Sprintf("RSA-%d", int(keySize)), func(t *testing.T) {
			t.Parallel()

			rsaPrivateKey, err := crypto.NewRSAPrikey(keySize)
			require.NoError(t, err)

			rsaPrivateKeyPEM, err := crypto.Prikey2Pem(rsaPrivateKey)
			require.NoError(t, err)

			rsaPublicKeyPEM, err := crypto.Pubkey2Pem(crypto.Prikey2Pubkey(rsaPrivateKey))
			require.NoError(t, err)

			// Test that all key sizes work with validation
			j, err := New(
				WithSignMethod(SignMethodRS256),
				WithPriKeyByte(rsaPrivateKeyPEM),
				WithPubKeyByte(rsaPublicKeyPEM),
			)
			require.NoError(t, err)
			require.NotNil(t, j)

			// Test that empty keys still fail regardless of key size
			_, err = New(
				WithSignMethod(SignMethodRS256),
				WithPriKeyByte([]byte("")),
				WithPubKeyByte(rsaPublicKeyPEM),
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "private key cannot be empty")
		})
	}
}
