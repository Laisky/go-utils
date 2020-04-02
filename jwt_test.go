package utils

import (
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/dgrijalva/jwt-go"
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
	jwt.StandardClaims
}

func ExampleJWT() {
	secret = []byte("4738947328rh3ru23f32hf238f238fh28f")
	j, err := NewJWT(
		WithJWTSignMethod(SignMethodHS256),
		WithJWTSecretByte(secret),
	)
	if err != nil {
		Logger.Panic("new jwt", zap.Error(err))
	}

	type jwtClaims struct {
		jwt.StandardClaims
	}

	claims := &jwtClaims{
		jwt.StandardClaims{
			Subject: "laisky",
		},
	}

	// signing
	token, err := j.Sign(claims)
	if err != nil {
		Logger.Panic("sign jwt", zap.Error(err))
	}

	// verify
	claims = &jwtClaims{}
	if err := j.ParseClaims(token, claims); err != nil {
		Logger.Panic("sign jwt", zap.Error(err))
	}

}

func TestJWTSignAndVerify(t *testing.T) {
	jwtES256, err := NewJWT(
		WithJWTSignMethod(SignMethodES256),
		WithJWTPubKeyByte(es256PubByte),
		WithJWTPriKeyByte(es256PriByte),
	)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	jwtHS256, err := NewJWT(
		WithJWTSignMethod(SignMethodHS256),
		WithJWTSecretByte(secret),
	)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	for _, j := range []*JWT{
		jwtES256,
		jwtHS256,
	} {

		claims := &testJWTClaims{
			StandardClaims: jwt.StandardClaims{
				Subject:  "laisky",
				Audience: "dune",
			},
		}

		// test sign & parse
		token, err := j.Sign(claims)
		if err != nil {
			t.Fatalf("generate token error %+v", err)
		}
		// expect := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJkdW5lIiwic3ViIjoibGFpc2t5In0.UtcJn1th7rvZNr0HLl6h5G8XE-sJLVSqyc96LYAFG42-p0ZAJJeDeE_9a5sp770hEaIXMtZSvVeeBQre90oTLA"
		// if token != expect {
		// 	t.Fatalf("expect %v,\n got %v", expect, token)
		// }

		claims = &testJWTClaims{}
		if err = j.ParseClaims(token, claims); err != nil {
			t.Fatalf("%+v", err)
		}
		if claims.Subject != "laisky" ||
			claims.Audience != "dune" {
			t.Fatal()
		}

		expired := Clock.GetUTCNow().Add(-time.Hour)
		future := Clock.GetUTCNow().Add(time.Hour)

		// test exp
		claims = &testJWTClaims{
			jwt.StandardClaims{
				ExpiresAt: expired.Unix(),
			},
		}
		claims.ExpiresAt = expired.Unix()
		if token, err = j.Sign(claims); err != nil {
			t.Fatalf("generate token error %+v", err)
		}
		if err = j.ParseClaims(token, claims); err != nil {
			if !strings.Contains(err.Error(), "token is expired") {
				t.Fatalf("must expired, got: %s", err.Error())
			}
		} else {
			t.Fatalf("must expired")
		}

		// test issuerAt
		claims = &testJWTClaims{
			jwt.StandardClaims{
				IssuedAt: future.Unix(),
			},
		}
		claims.ExpiresAt = expired.Unix()
		if token, err = j.Sign(claims); err != nil {
			t.Fatalf("generate token error %+v", err)
		}
		if err = j.ParseClaims(token, claims); err != nil {
			if !strings.Contains(err.Error(), "used before issued") {
				t.Fatalf("must invalid, got: %s", err.Error())
			}
		} else {
			t.Fatalf("must invalid")
		}
	}
}

func TestParseJWTTokenWithoutValidate(t *testing.T) {
	token := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJkdW5lIiwic3ViIjoibGFpc2t5In0.UtcJn1th7rvZNr0HLl6h5G8XE-sJLVSqyc96LYAFG42-p0ZAJJeDeE_9a5sp770hEaIXMtZSvVeeBQre90oTLA"
	claims, err := ParseJWTTokenWithoutValidate(token)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if claims["sub"] != "laisky" ||
		claims["aud"] != "dune" {
		t.Fatal()
	}
}
