package jwt

import (
	stderrors "errors"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
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

// testJWTClaims is the claims type the tests sign and parse; it only embeds jwt.RegisteredClaims.
type testJWTClaims struct {
	jwt.RegisteredClaims
}

// requireAudienceValidation validates claims with a jwt/v5 validator that requires audience. When wantErr is nil it
// asserts that validation succeeds; otherwise it asserts that validation fails with an error matching wantErr through
// errors.Is. Any mismatch fails the test t.
func requireAudienceValidation(t *testing.T, claims jwt.Claims, audience string, wantErr error) {
	t.Helper()

	err := jwt.NewValidator(jwt.WithAudience(audience)).Validate(claims)
	if wantErr == nil {
		require.NoError(t, err)
		return
	}

	require.Error(t, err)
	require.True(t, stderrors.Is(err, wantErr), "unexpected error: %v", err)
}

// ExampleJWT demonstrates creating an HS256 JWT helper from a shared secret, signing registered claims that carry a
// subject, and parsing the signed token back into a claims struct.
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

// TestJWTSignAndVerify verifies, for both an ES256 and an HS256 instance, that signed claims parse back with the same
// subject and audience, that an expired token is rejected with jwt.ErrTokenExpired, and that a token issued in the
// future is rejected with jwt.ErrTokenUsedBeforeIssued.
func TestJWTSignAndVerify(t *testing.T) {
	t.Parallel()

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

		expired := time.Now().UTC().Add(-time.Hour)
		future := time.Now().UTC().Add(time.Hour)

		// test exp
		claims = &testJWTClaims{
			jwt.RegisteredClaims{
				ExpiresAt: &jwt.NumericDate{Time: expired},
			},
		}
		if token, err = j.Sign(claims); err != nil {
			require.NoError(t, err, "generate token error %+v", err)
		}
		err = j.ParseClaims(token, claims)
		require.Error(t, err)
		require.True(t, stderrors.Is(err, jwt.ErrTokenExpired), "must expired, got: %v", err)

		// test issuerAt
		claims = &testJWTClaims{
			jwt.RegisteredClaims{
				IssuedAt:  &jwt.NumericDate{Time: future},
				ExpiresAt: &jwt.NumericDate{Time: future.Add(time.Hour)},
			},
		}
		if token, err = j.Sign(claims); err != nil {
			require.NoError(t, err, "generate token error %+v", err)
		}
		err = j.ParseClaims(token, claims)
		require.Error(t, err)
		require.True(t, stderrors.Is(err, jwt.ErrTokenUsedBeforeIssued), "must be used before issued, got: %v", err)
	}
}

// TestParseJWTTokenWithoutValidate verifies that ParseTokenWithoutValidate decodes the subject and the array audience
// of a fixed HS256 token without checking its signature.
func TestParseJWTTokenWithoutValidate(t *testing.T) {
	t.Parallel()

	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZHVuZSJdLCJzdWIiOiJsYWlza3kifQ.cYnd2OdN-i3kuPXSUc4xj1rkVk5elJnxln6zDdvlOUc"

	c := new(jwt.RegisteredClaims)
	err := ParseTokenWithoutValidate(token, c)
	require.NoError(t, err)
	require.Equal(t, "laisky", c.Subject)
	require.Equal(t, jwt.ClaimStrings([]string{"dune"}), c.Audience)
}

// TestJWTAudValunerable guards against the jwt-go audience bypass in which an array-valued aud claim skipped audience
// verification. For a token whose aud is ["dune", "laisky"] it checks that either listed audience is accepted and an
// empty expected audience is rejected with jwt.ErrTokenInvalidAudience, both after ParseClaims and after
// ParseTokenWithoutValidate.
//
// References:
//
// https://snyk.io/vuln/SNYK-GOLANG-GITHUBCOMDGRIJALVAJWTGO-596515?utm_medium=Partner&utm_source=RedHat&utm_campaign=Code-Ready-Analytics-2020&utm_content=vuln/SNYK-GOLANG-GITHUBCOMDGRIJALVAJWTGO-596515
// https://github.com/dgrijalva/jwt-go/issues/422
func TestJWTAudValunerable(t *testing.T) {
	t.Parallel()

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

		requireAudienceValidation(t, claims, "laisky", nil)
		requireAudienceValidation(t, claims, "dune", nil)
		requireAudienceValidation(t, claims, "", jwt.ErrTokenInvalidAudience)
	}

	// bug: slice aud will bypass verify
	{
		claims := new(jwt.RegisteredClaims)
		err := ParseTokenWithoutValidate(token, claims)
		require.NoError(t, err)

		requireAudienceValidation(t, claims, "laisky", nil)
		requireAudienceValidation(t, claims, "dune", nil)
		requireAudienceValidation(t, claims, "", jwt.ErrTokenInvalidAudience)
	}
}

// TestParseTokenWithoutValidateStillWorks verifies that a token signed with SignByHS256 is still decoded by
// ParseTokenWithoutValidate, which must stay unaffected by the key validation performed by New and the divide options.
func TestParseTokenWithoutValidateStillWorks(t *testing.T) {
	t.Parallel()

	// Ensure that ParseTokenWithoutValidate still works regardless of our validation changes

	// Create a token
	j, err := New(
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

	token, err := j.SignByHS256(claims)
	require.NoError(t, err)

	// Parse without validation - this should always work
	parsedClaims := &testJWTClaims{}
	err = ParseTokenWithoutValidate(token, parsedClaims)
	require.NoError(t, err)
	require.Equal(t, "test-user", parsedClaims.Subject)
}

// TestHS256RejectsNilSecret verifies that an HS256 instance created without a secret refuses to sign through both
// SignByHS256 and the Sign dispatcher with an "HS256 secret must not be empty" error, so it never issues tokens that
// are forgeable with an empty HMAC key.
func TestHS256RejectsNilSecret(t *testing.T) {
	t.Parallel()

	// A JWT instance created without a secret must not silently sign/verify
	// with an empty HMAC key, as that would produce trivially forgeable tokens.
	j, err := New(WithSignMethod(SignMethodHS256))
	require.NoError(t, err)

	claims := &testJWTClaims{
		jwt.RegisteredClaims{
			Subject:   "test",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	// Signing with nil secret must fail
	_, err = j.SignByHS256(claims)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HS256 secret must not be empty")

	// Signing via Sign() dispatcher must also fail
	_, err = j.Sign(claims)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HS256 secret must not be empty")
}
