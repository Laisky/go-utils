package jwt

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/crypto"
	"github.com/Laisky/go-utils/v5/log"
)

var (
	es256PriByte = []byte(`-----BEGIN PRIVATE KEY-----
MHcCAQEEIKBr4xv3gD85+ZAfgflb6y36PEwQjA+fD4w7QjIlxoD0oAoGCCqGSM49
AwEHoUQDQgAEUfNN1nvU2g8yr058Fsvjx6k6sOdcqLW+xXwTysxo/xiZcW8fwQow
CyxcGJv8r7OfHYB/FScm3jgOaNhabM6laQ==
-----END PRIVATE KEY-----`)
	es256PubByte = []byte(`-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEUfNN1nvU2g8yr058Fsvjx6k6sOdc
qLW+xXwTysxo/xiZcW8fwQowCyxcGJv8r7OfHYB/FScm3jgOaNhabM6laQ==
-----END PUBLIC KEY-----
`)
	secret = []byte("4738947328rh3ru23f32hf238f238fh28f")
)

type testJWTClaims struct {
	jwt.RegisteredClaims
}

func ExampleJWT() {
	secret = []byte("4738947328rh3ru23f32hf238f238fh28f")
	j, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte(secret),
	)
	if err != nil {
		log.Shared.Panic("new jwt", zap.Error(err))
	}

	type jwtClaims struct {
		jwt.RegisteredClaims
	}

	claims := &jwtClaims{
		jwt.RegisteredClaims{
			Subject: "laisky",
		},
	}

	// signing
	token, err := j.Sign(claims)
	if err != nil {
		log.Shared.Panic("sign jwt", zap.Error(err))
	}

	// verify
	claims = &jwtClaims{}
	if err := j.ParseClaims(token, claims); err != nil {
		log.Shared.Panic("sign jwt", zap.Error(err))
	}
}

func TestJWTSignAndVerify(t *testing.T) {
	jwtES256, err := New(
		WithSignMethod(SignMethodES256),
		WithPubKeyByte(es256PubByte),
		WithPriKeyByte(es256PriByte),
	)
	require.NoError(t, err)

	jwtHS256, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte(secret),
	)
	require.NoError(t, err)

	for _, j := range []JWT{
		jwtES256,
		jwtHS256,
	} {

		claims := &testJWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:  "laisky",
				Audience: []string{"dune"},
			},
		}

		// test sign & parse
		token, err := j.Sign(claims)
		require.NoError(t, err)

		// expect := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJkdW5lIiwic3ViIjoibGFpc2t5In0.UtcJn1th7rvZNr0HLl6h5G8XE-sJLVSqyc96LYAFG42-p0ZAJJeDeE_9a5sp770hEaIXMtZSvVeeBQre90oTLA"
		// if token != expect {
		// 	t.Fatalf("expect %v,\n got %v", expect, token)
		// }

		claims = &testJWTClaims{}
		if err = j.ParseClaims(token, claims); err != nil {
			require.NoError(t, err, "%+v", err)
		}
		if claims.Subject != "laisky" ||
			claims.Audience[0] != "dune" {
			t.Fatal()
		}

		expired := gutils.Clock.GetUTCNow().Add(-time.Hour)
		future := gutils.Clock.GetUTCNow().Add(time.Hour)

		// test exp
		claims = &testJWTClaims{
			jwt.RegisteredClaims{
				ExpiresAt: &jwt.NumericDate{Time: expired},
			},
		}
		claims.ExpiresAt = &jwt.NumericDate{Time: expired}
		if token, err = j.Sign(claims); err != nil {
			require.NoError(t, err, "generate token error %+v", err)
		}
		if err = j.ParseClaims(token, claims); err != nil {
			if !strings.Contains(err.Error(), "token is expired") {
				require.NoError(t, err, "must expired, got: %s", err.Error())
			}
		} else {
			require.NoError(t, err, "must expired")
		}

		// test issuerAt
		claims = &testJWTClaims{
			jwt.RegisteredClaims{
				IssuedAt: &jwt.NumericDate{Time: future},
			},
		}
		claims.ExpiresAt = &jwt.NumericDate{Time: expired}
		if token, err = j.Sign(claims); err != nil {
			require.NoError(t, err, "generate token error %+v", err)
		}
		if err = j.ParseClaims(token, claims); err != nil {
			if !strings.Contains(err.Error(), "used before issued") {
				require.NoError(t, err, "must invalid, got: %s", err.Error())
			}
		} else {
			require.NoError(t, err, "must invalid")
		}
	}
}

func TestParseJWTTokenWithoutValidate(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZHVuZSJdLCJzdWIiOiJsYWlza3kifQ.cYnd2OdN-i3kuPXSUc4xj1rkVk5elJnxln6zDdvlOUc"

	c := new(jwt.RegisteredClaims)
	err := ParseTokenWithoutValidate(token, c)
	require.NoError(t, err)
	require.Equal(t, "laisky", c.Subject)
	require.Equal(t, jwt.ClaimStrings([]string{"dune"}), c.Audience)
}

// https://snyk.io/vuln/SNYK-GOLANG-GITHUBCOMDGRIJALVAJWTGO-596515?utm_medium=Partner&utm_source=RedHat&utm_campaign=Code-Ready-Analytics-2020&utm_content=vuln/SNYK-GOLANG-GITHUBCOMDGRIJALVAJWTGO-596515
// https://github.com/dgrijalva/jwt-go/issues/422
func TestJWTAudValunerable(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYXVkIjpbImR1bmUiLCJsYWlza3kiXSwiaWF0IjoxNTE2MjM5MDIyfQ.lmil648BC0ZqwPZQDctuTvu-R6w4mDWnvsmWsqEtxv4"

	// case: v3 的 aud 是 stirng，应该无法解析 []string
	{
		j, err := New(
			WithSignMethod(SignMethodHS256),
			WithSecretByte(secret),
		)
		require.NoError(t, err)
		claims := new(jwt.RegisteredClaims)
		err = j.ParseClaims(token, claims)
		require.NoError(t, err)

		ok := claims.VerifyAudience("laisky", false)
		require.True(t, ok)

		ok = claims.VerifyAudience("dune", false)
		require.True(t, ok)

		ok = claims.VerifyAudience("", false)
		require.False(t, ok)
	}

	// bug: slice aud will bypass verify
	{
		claims := new(jwt.RegisteredClaims)
		err := ParseTokenWithoutValidate(token, claims)
		require.NoError(t, err)

		ok := claims.VerifyAudience("laisky", false)
		require.True(t, ok)

		ok = claims.VerifyAudience("dune", false)
		require.True(t, ok)

		ok = claims.VerifyAudience("", false)
		require.False(t, ok)
	}
}

func TestWithSecretByteValidation(t *testing.T) {
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

	// Test that valid secret works
	_, err = New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("valid-secret")),
	)
	require.NoError(t, err)
}

func TestWithPriKeyByteValidation(t *testing.T) {
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
	_, err = j.SignByHS256(claims, WithDivideSecret([]byte("valid-divide-secret")))
	require.NoError(t, err)
}

func TestParseWithDivideOptionsOnly(t *testing.T) {
	// Test that we can create JWT instance without main keys and use divide options
	// This should work for parsing tokens where keys are provided via divide options

	// First, create a token with a JWT that has keys
	j1, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("test-secret")),
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
	err = j2.ParseClaimsByHS256(token, parsedClaims, WithDivideSecret([]byte("test-secret")))
	require.NoError(t, err)
	require.Equal(t, "test-user", parsedClaims.Subject)
}

func TestParseWithoutKeysFailsGracefully(t *testing.T) {
	// Test that parsing without any keys fails gracefully with a meaningful error

	// Create a token first
	j1, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("test-secret")),
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
}

func TestParseTokenWithoutValidateStillWorks(t *testing.T) {
	// Ensure that ParseTokenWithoutValidate still works regardless of our validation changes

	// Create a token
	j, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("test-secret")),
	)
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := j.SignByHS256(claims)
	require.NoError(t, err)

	// Parse without validation - this should always work
	parsedClaims := &testJWTClaims{}
	err = ParseTokenWithoutValidate(token, parsedClaims)
	require.NoError(t, err)
	require.Equal(t, "test-user", parsedClaims.Subject)
}

func TestRS256ParsingValidation(t *testing.T) {
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
	// Test combinations of valid and invalid options

	// Test valid secret with invalid divide secret
	j, err := New(
		WithSignMethod(SignMethodHS256),
		WithSecretByte([]byte("valid-main-secret")),
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

func TestValidationWithAllSigningMethods(t *testing.T) {
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
			validOptions:   []Option{WithSecretByte([]byte("valid-secret"))},
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

func TestComprehensiveKeyValidationWithGeneratedKeys(t *testing.T) {
	// Test with generated RSA keys
	t.Run("RSA Keys", func(t *testing.T) {
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

func TestDivideOptionsWithGeneratedKeys(t *testing.T) {
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

func TestKeyValidationWithDifferentKeySizes(t *testing.T) {
	// Test validation works with different RSA key sizes
	keySizes := []crypto.RSAPrikeyBits{
		crypto.RSAPrikeyBits2048,
		crypto.RSAPrikeyBits3072,
		crypto.RSAPrikeyBits4096,
	}

	for _, keySize := range keySizes {
		t.Run(fmt.Sprintf("RSA-%d", int(keySize)), func(t *testing.T) {
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
