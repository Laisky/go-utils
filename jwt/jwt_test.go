package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/go-utils/v3/log"
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
