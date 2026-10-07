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

type testJWTClaims struct {
	jwt.RegisteredClaims
}

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

func TestParseJWTTokenWithoutValidate(t *testing.T) {
	t.Parallel()

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
