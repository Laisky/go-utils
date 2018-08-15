package utils

// JWT payload should looks like:
//
// ```js
// {
// 	"k1": "v1",
// 	"k2": "v2",
// 	"k3": "v3",
// 	"username": "laisky"
// }
// ```
//
// and the payload would be looks like:
//
// ```js
// {
// 	"expires_at": "2286-11-20T17:46:40Z",
// 	"k1": "v1",
// 	"k2": "v2",
// 	"k3": "v3",
// 	"username": "laisky"
//   }
// ```

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/pkg/errors"

	jwt "github.com/dgrijalva/jwt-go"
)

// JWT struct to generate and validate jwt tokens
type JWT struct {
	secret      []byte
	layout      string
	TKExpiresAt string
	TKUsername  string
}

// Setup initialize JWT
func (j *JWT) Setup(secret string) {
	// const key names
	j.TKExpiresAt = "expires_at"
	j.TKUsername = "username"

	j.secret = []byte(secret)
	j.layout = time.RFC3339
}

// Generate generate JWT token
func (j *JWT) Generate(expiresAt int64, payload map[string]interface{}) (string, error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload["expires_at"] = ParseTs2String(expiresAt, j.layout)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtPayload)
	tokenStr, err := token.SignedString(j.secret)
	if err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// CheckExpiresValid return the bool whether the `expires_at` is not expired
func (j *JWT) CheckExpiresValid(now time.Time, expiresAtI interface{}) (ok bool, err error) {
	expiresAt, ok := expiresAtI.(string)
	if !ok {
		return false, fmt.Errorf("`%v` is not string", j.TKExpiresAt)
	}
	tokenT, err := time.Parse(j.layout, expiresAt)
	if err != nil {
		return false, errors.Wrap(err, "try to parse token expires_at error")
	}

	return now.Before(tokenT), nil
}

// Validate validate the token and return the payload
func (j *JWT) Validate(tokenStr string) (payload map[string]interface{}, err error) {
	Logger.Debug("Validate for token", zap.String("tokenStr", tokenStr))
	payload = map[string]interface{}{}
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("JWT method not allowd")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "token validate error")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		for k, v := range claims {
			payload[k] = v
		}
		if _, ok := payload[j.TKUsername]; !ok {
			return payload, fmt.Errorf("token do not contains `%v`", j.TKUsername)
		}

		if expiresAt, ok := payload[j.TKExpiresAt]; !ok {
			return payload, fmt.Errorf("token do not contains `%v`", j.TKExpiresAt)
		} else {
			if ok, err = j.CheckExpiresValid(UTCNow(), expiresAt); err != nil {
				return payload, errors.Wrap(err, "parse token `expires_at` error")
			} else if !ok {
				return payload, fmt.Errorf("token expired at %v", payload[j.TKExpiresAt])
			}
		}

		return payload, nil
	}
	return nil, errors.New("token not match MapClaims")
}
